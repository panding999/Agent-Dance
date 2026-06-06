package ast

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"nhooyr.io/websocket"
)

const (
	DefaultEndpoint   = "wss://openspeech.bytedance.com/api/v4/ast/v2/translate"
	DefaultResourceID = "volc.service_type.10053"

	HeaderAPIKey     = "X-Api-Key"
	HeaderAppID      = "X-Api-App-Id"
	HeaderAppKey     = "X-Api-App-Key"
	HeaderAccessKey  = "X-Api-Access-Key"
	HeaderResourceID = "X-Api-Resource-Id"
	HeaderLogID      = "X-Tt-Logid"
)

var (
	ErrMissingCredentials    = errors.New("missing doubao credentials")
	ErrMissingAppKey         = ErrMissingCredentials
	ErrMissingCodec          = errors.New("missing ast codec")
	ErrSessionAlreadyStarted = errors.New("ast session already started")
	ErrSessionNotStarted     = errors.New("ast session is not started")
)

type ClientOptions struct {
	Endpoint   string
	APIKey     string
	AppID      string
	AppKey     string
	AccessKey  string
	ResourceID string
	ModelID    string
	Codec      Codec
	HTTPClient *http.Client
}

type Client struct {
	endpoint   string
	apiKey     string
	appID      string
	appKey     string
	accessKey  string
	resourceID string
	modelID    string
	codec      Codec
	httpClient *http.Client

	mu         sync.Mutex
	writeMu    sync.Mutex
	conn       *websocket.Conn
	logID      string
	sessionID  string
	ready      chan struct{}
	readyDone  bool
	readCancel context.CancelFunc

	events chan ProviderEvent
	errors chan error
}

func NewClient(options ClientOptions) (*Client, error) {
	apiKey := strings.TrimSpace(options.APIKey)
	appID := strings.TrimSpace(options.AppID)
	appKey := strings.TrimSpace(options.AppKey)
	accessKey := strings.TrimSpace(options.AccessKey)
	if !hasCredentials(apiKey, appID, appKey, accessKey) {
		return nil, ErrMissingCredentials
	}
	if options.Codec == nil {
		return nil, ErrMissingCodec
	}

	endpoint := strings.TrimSpace(options.Endpoint)
	if endpoint == "" {
		endpoint = DefaultEndpoint
	}
	resourceID := strings.TrimSpace(options.ResourceID)
	if resourceID == "" {
		resourceID = DefaultResourceID
	}

	return &Client{
		endpoint:   endpoint,
		apiKey:     apiKey,
		appID:      appID,
		appKey:     appKey,
		accessKey:  accessKey,
		resourceID: resourceID,
		modelID:    strings.TrimSpace(options.ModelID),
		codec:      options.Codec,
		httpClient: options.HTTPClient,
		events:     make(chan ProviderEvent, 32),
		errors:     make(chan error, 4),
	}, nil
}

func (c *Client) Endpoint() string {
	return c.endpoint
}

func (c *Client) ResourceID() string {
	return c.resourceID
}

func (c *Client) ModelID() string {
	return c.modelID
}

func (c *Client) LogID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logID
}

func (c *Client) StartSession(ctx context.Context, params StartSessionParams) error {
	if strings.TrimSpace(params.ModelID) == "" {
		params.ModelID = c.modelID
	}
	req, err := newStartSessionRequest(params)
	if err != nil {
		return err
	}
	payload, err := c.codec.EncodeStartSession(req)
	if err != nil {
		return fmt.Errorf("encode ast start session: %w", err)
	}

	c.mu.Lock()
	if c.conn != nil {
		c.mu.Unlock()
		return ErrSessionAlreadyStarted
	}
	ready := make(chan struct{})
	readCtx, readCancel := context.WithCancel(context.Background())
	c.mu.Unlock()

	dialOptions := &websocket.DialOptions{HTTPHeader: c.authHeaders()}
	if c.httpClient != nil {
		dialOptions.HTTPClient = c.httpClient
	}

	conn, resp, err := websocket.Dial(ctx, c.endpoint, dialOptions)
	if err != nil {
		readCancel()
		return fmt.Errorf("dial ast websocket: %w", err)
	}
	logID := ""
	if resp != nil {
		logID = resp.Header.Get(HeaderLogID)
	}

	if err := c.writeBinary(ctx, conn, payload); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "start session failed")
		readCancel()
		return fmt.Errorf("write ast start session: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.logID = logID
	c.sessionID = req.RequestMeta.SessionID
	c.ready = ready
	c.readyDone = false
	c.readCancel = readCancel
	c.mu.Unlock()

	go c.readLoop(readCtx, conn, ready)

	return nil
}

func (c *Client) SendAudio(ctx context.Context, packet audio.Packet) error {
	conn, ready, err := c.connectionWithReady()
	if err != nil {
		return err
	}
	select {
	case <-ready:
	case <-ctx.Done():
		return ctx.Err()
	}

	req := newTaskRequest(packet)
	payload, err := c.codec.EncodeTaskRequest(req)
	if err != nil {
		return fmt.Errorf("encode ast task request: %w", err)
	}

	if err := c.writeBinary(ctx, conn, payload); err != nil {
		return fmt.Errorf("write ast task request: %w", err)
	}
	return nil
}

func (c *Client) FinishSession(ctx context.Context) error {
	conn, sessionID, err := c.connectionWithSession()
	if err != nil {
		return err
	}

	payload, err := c.codec.EncodeFinishSession(newFinishSessionRequest(sessionID))
	if err != nil {
		return fmt.Errorf("encode ast finish session: %w", err)
	}
	if err := c.writeBinary(ctx, conn, payload); err != nil {
		return fmt.Errorf("write ast finish session: %w", err)
	}
	if err := conn.Close(websocket.StatusNormalClosure, "session finished"); err != nil {
		return fmt.Errorf("close ast websocket: %w", err)
	}

	c.clearConnection(conn)

	return nil
}

func (c *Client) Close(code websocket.StatusCode, reason string) error {
	c.mu.Lock()
	conn := c.conn
	cancel := c.readCancel
	c.conn = nil
	c.sessionID = ""
	c.ready = nil
	c.readyDone = false
	c.readCancel = nil
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if conn == nil {
		return nil
	}
	return conn.Close(code, reason)
}

func (c *Client) Events() <-chan ProviderEvent {
	return c.events
}

func (c *Client) Errors() <-chan error {
	return c.errors
}

func (c *Client) connection() (*websocket.Conn, error) {
	conn, _, err := c.connectionWithSession()
	return conn, err
}

func (c *Client) connectionWithReady() (*websocket.Conn, <-chan struct{}, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil || c.ready == nil {
		return nil, nil, ErrSessionNotStarted
	}
	return c.conn, c.ready, nil
}

func (c *Client) connectionWithSession() (*websocket.Conn, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil, "", ErrSessionNotStarted
	}
	return c.conn, c.sessionID, nil
}

func (c *Client) authHeaders() http.Header {
	headers := http.Header{}
	headers.Set(HeaderResourceID, c.resourceID)

	if c.apiKey != "" {
		headers.Set(HeaderAPIKey, c.apiKey)
		return headers
	}
	if c.appID != "" {
		headers.Set(HeaderAppID, c.appID)
	}
	if c.appKey != "" {
		headers.Set(HeaderAppKey, c.appKey)
	}
	if c.accessKey != "" {
		headers.Set(HeaderAccessKey, c.accessKey)
	}
	return headers
}

func (c *Client) writeBinary(ctx context.Context, conn *websocket.Conn, payload []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return conn.Write(ctx, websocket.MessageBinary, payload)
}

func (c *Client) readLoop(ctx context.Context, conn *websocket.Conn, ready chan struct{}) {
	defer c.clearConnection(conn)

	for {
		messageType, payload, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			status := websocket.CloseStatus(err)
			if status == websocket.StatusNormalClosure || status == websocket.StatusGoingAway {
				return
			}
			c.publishError(fmt.Errorf("read ast provider event: %w", err))
			return
		}
		if messageType != websocket.MessageBinary {
			continue
		}

		event, err := c.codec.DecodeProviderEvent(payload)
		if err != nil {
			c.publishError(fmt.Errorf("decode ast provider event: %w", err))
			continue
		}
		if event.Event == EventSessionStarted {
			c.markReady(ready)
		}
		c.publishEvent(event)
	}
}

func (c *Client) markReady(ready chan struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ready != ready || c.readyDone {
		return
	}
	close(ready)
	c.readyDone = true
}

func (c *Client) clearConnection(conn *websocket.Conn) {
	c.mu.Lock()
	var cancel context.CancelFunc
	if c.conn == conn {
		cancel = c.readCancel
		c.conn = nil
		c.sessionID = ""
		c.ready = nil
		c.readyDone = false
		c.readCancel = nil
	}
	c.mu.Unlock()

	if cancel != nil {
		cancel()
	}
}

func (c *Client) publishEvent(event ProviderEvent) {
	select {
	case c.events <- event:
	default:
		c.publishError(errors.New("ast provider event buffer is full"))
	}
}

func (c *Client) publishError(err error) {
	if err == nil {
		return
	}
	select {
	case c.errors <- err:
	default:
	}
}

func hasCredentials(apiKey string, appID string, appKey string, accessKey string) bool {
	return apiKey != "" || appKey != "" || (appID != "" && accessKey != "")
}
