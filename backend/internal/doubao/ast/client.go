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

	HeaderAppKey     = "X-Api-App-Key"
	HeaderResourceID = "X-Api-Resource-Id"
	HeaderLogID      = "X-Tt-Logid"
)

var (
	ErrMissingAppKey         = errors.New("missing doubao app key")
	ErrMissingCodec          = errors.New("missing ast codec")
	ErrSessionAlreadyStarted = errors.New("ast session already started")
	ErrSessionNotStarted     = errors.New("ast session is not started")
)

type ClientOptions struct {
	Endpoint   string
	AppKey     string
	ResourceID string
	Codec      Codec
	HTTPClient *http.Client
}

type Client struct {
	endpoint   string
	appKey     string
	resourceID string
	codec      Codec
	httpClient *http.Client

	mu        sync.Mutex
	conn      *websocket.Conn
	logID     string
	sessionID string
}

func NewClient(options ClientOptions) (*Client, error) {
	appKey := strings.TrimSpace(options.AppKey)
	if appKey == "" {
		return nil, ErrMissingAppKey
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
		appKey:     appKey,
		resourceID: resourceID,
		codec:      options.Codec,
		httpClient: options.HTTPClient,
	}, nil
}

func (c *Client) Endpoint() string {
	return c.endpoint
}

func (c *Client) ResourceID() string {
	return c.resourceID
}

func (c *Client) LogID() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.logID
}

func (c *Client) StartSession(ctx context.Context, params StartSessionParams) error {
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
	c.mu.Unlock()

	headers := http.Header{}
	headers.Set(HeaderAppKey, c.appKey)
	headers.Set(HeaderResourceID, c.resourceID)
	dialOptions := &websocket.DialOptions{HTTPHeader: headers}
	if c.httpClient != nil {
		dialOptions.HTTPClient = c.httpClient
	}

	conn, resp, err := websocket.Dial(ctx, c.endpoint, dialOptions)
	if err != nil {
		return fmt.Errorf("dial ast websocket: %w", err)
	}
	logID := ""
	if resp != nil {
		logID = resp.Header.Get(HeaderLogID)
	}

	if err := conn.Write(ctx, websocket.MessageBinary, payload); err != nil {
		_ = conn.Close(websocket.StatusInternalError, "start session failed")
		return fmt.Errorf("write ast start session: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.logID = logID
	c.sessionID = req.RequestMeta.SessionID
	c.mu.Unlock()

	return nil
}

func (c *Client) SendAudio(ctx context.Context, packet audio.Packet) error {
	req := newTaskRequest(packet)
	payload, err := c.codec.EncodeTaskRequest(req)
	if err != nil {
		return fmt.Errorf("encode ast task request: %w", err)
	}

	conn, err := c.connection()
	if err != nil {
		return err
	}
	if err := conn.Write(ctx, websocket.MessageBinary, payload); err != nil {
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
	if err := conn.Write(ctx, websocket.MessageBinary, payload); err != nil {
		return fmt.Errorf("write ast finish session: %w", err)
	}
	if err := conn.Close(websocket.StatusNormalClosure, "session finished"); err != nil {
		return fmt.Errorf("close ast websocket: %w", err)
	}

	c.mu.Lock()
	if c.conn == conn {
		c.conn = nil
		c.sessionID = ""
	}
	c.mu.Unlock()

	return nil
}

func (c *Client) Close(code websocket.StatusCode, reason string) error {
	c.mu.Lock()
	conn := c.conn
	c.conn = nil
	c.sessionID = ""
	c.mu.Unlock()

	if conn == nil {
		return nil
	}
	return conn.Close(code, reason)
}

func (c *Client) connection() (*websocket.Conn, error) {
	conn, _, err := c.connectionWithSession()
	return conn, err
}

func (c *Client) connectionWithSession() (*websocket.Conn, string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil, "", ErrSessionNotStarted
	}
	return c.conn, c.sessionID, nil
}
