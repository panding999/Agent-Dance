package ast

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"nhooyr.io/websocket"
)

func TestClientDefaultsEndpointAndResourceID(t *testing.T) {
	client, err := NewClient(ClientOptions{
		APIKey: "api-key",
		Codec:  &recordingCodec{},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	if client.Endpoint() != DefaultEndpoint {
		t.Fatalf("Endpoint() = %q, want %q", client.Endpoint(), DefaultEndpoint)
	}
	if client.ResourceID() != DefaultResourceID {
		t.Fatalf("ResourceID() = %q, want %q", client.ResourceID(), DefaultResourceID)
	}
}

func TestNewClientRejectsMissingRequiredOptions(t *testing.T) {
	_, err := NewClient(ClientOptions{Codec: &recordingCodec{}})
	if !errors.Is(err, ErrMissingCredentials) {
		t.Fatalf("NewClient() error = %v, want ErrMissingCredentials", err)
	}

	_, err = NewClient(ClientOptions{APIKey: "api-key"})
	if !errors.Is(err, ErrMissingCodec) {
		t.Fatalf("NewClient() error = %v, want ErrMissingCodec", err)
	}
}

func TestClientStartSessionDialsWithAPIKeyCapturesLogIDAndSendsStartRequest(t *testing.T) {
	server := newFakeASTServer(t)
	codec := &recordingCodec{}
	client, err := NewClient(ClientOptions{
		Endpoint: server.endpoint,
		APIKey:   "api-key",
		ModelID:  "model-id",
		Codec:    codec,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err = client.StartSession(ctx, StartSessionParams{
		SessionID:      "session-1",
		Mode:           ModeS2T,
		SourceLanguage: "en",
		TargetLanguage: "zh",
		Corpus: Corpus{
			HotWordsList: []string{"RAG"},
			GlossaryList: map[string]string{"agent": "智能体"},
		},
	})
	if err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close(websocket.StatusNormalClosure, "test done")
	})

	headers := server.readHeaders(t)
	if headers.Get(HeaderAPIKey) != "api-key" {
		t.Fatalf("%s = %q, want api-key", HeaderAPIKey, headers.Get(HeaderAPIKey))
	}
	if headers.Get(HeaderResourceID) != DefaultResourceID {
		t.Fatalf("%s = %q, want %q", HeaderResourceID, headers.Get(HeaderResourceID), DefaultResourceID)
	}
	if client.LogID() != "log-123" {
		t.Fatalf("LogID() = %q, want log-123", client.LogID())
	}

	gotRequest := codec.startRequests[0]
	if gotRequest.Event != EventStartSession {
		t.Fatalf("start event = %d, want %d", gotRequest.Event, EventStartSession)
	}
	if gotRequest.RequestMeta.SessionID != "session-1" {
		t.Fatalf("session_id = %q, want session-1", gotRequest.RequestMeta.SessionID)
	}
	if gotRequest.RequestMeta.ModelID != "model-id" {
		t.Fatalf("model_id = %q, want model-id", gotRequest.RequestMeta.ModelID)
	}
	if gotRequest.Request.Mode != ModeS2T {
		t.Fatalf("mode = %q, want %q", gotRequest.Request.Mode, ModeS2T)
	}
	if gotRequest.Request.SourceLanguage != "en" || gotRequest.Request.TargetLanguage != "zh" {
		t.Fatalf("language pair = %q -> %q, want en -> zh", gotRequest.Request.SourceLanguage, gotRequest.Request.TargetLanguage)
	}
	if gotRequest.SourceAudio != DefaultSourceAudioConfig() {
		t.Fatalf("source_audio = %+v, want %+v", gotRequest.SourceAudio, DefaultSourceAudioConfig())
	}
	if len(gotRequest.Request.Corpus.HotWordsList) != 1 || gotRequest.Request.Corpus.HotWordsList[0] != "RAG" {
		t.Fatalf("hot_words_list = %#v", gotRequest.Request.Corpus.HotWordsList)
	}
	if gotRequest.Request.Corpus.GlossaryList["agent"] != "智能体" {
		t.Fatalf("glossary_list = %#v", gotRequest.Request.Corpus.GlossaryList)
	}

	payload := server.readPayload(t)
	if !bytes.Equal(payload, []byte("start:session-1")) {
		t.Fatalf("start payload = %q, want %q", payload, []byte("start:session-1"))
	}
}

func TestClientStartSessionDialsWithLegacyAppHeaders(t *testing.T) {
	server := newFakeASTServer(t)
	client, err := NewClient(ClientOptions{
		Endpoint:  server.endpoint,
		AppID:     "app-id",
		AppKey:    "app-key",
		AccessKey: "access-key",
		Codec:     &recordingCodec{},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close(websocket.StatusNormalClosure, "test done")
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := client.StartSession(ctx, StartSessionParams{
		SessionID:      "session-legacy",
		Mode:           ModeS2T,
		SourceLanguage: "en",
		TargetLanguage: "zh",
	}); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}

	headers := server.readHeaders(t)
	if headers.Get(HeaderAppID) != "app-id" {
		t.Fatalf("%s = %q, want app-id", HeaderAppID, headers.Get(HeaderAppID))
	}
	if headers.Get(HeaderAppKey) != "app-key" {
		t.Fatalf("%s = %q, want app-key", HeaderAppKey, headers.Get(HeaderAppKey))
	}
	if headers.Get(HeaderAccessKey) != "access-key" {
		t.Fatalf("%s = %q, want access-key", HeaderAccessKey, headers.Get(HeaderAccessKey))
	}
}

func TestClientSendAudioAndFinishSessionWriteTaskAndFinishRequests(t *testing.T) {
	server := newFakeASTServer(t)
	codec := &recordingCodec{}
	client, err := NewClient(ClientOptions{
		Endpoint: server.endpoint,
		APIKey:   "api-key",
		Codec:    codec,
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close(websocket.StatusNormalClosure, "test done")
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := client.StartSession(ctx, StartSessionParams{
		SessionID:      "session-2",
		Mode:           ModeS2T,
		SourceLanguage: "en",
		TargetLanguage: "zh",
	}); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	_ = server.readHeaders(t)
	_ = server.readPayload(t)
	server.writeProviderPayload(t, []byte("session-started"))
	_ = readClientEvent(t, client)

	packet := audio.Packet{
		Sequence:    7,
		TimestampMS: 560,
		PCM:         []byte{1, 0, 2, 0},
	}
	if err := client.SendAudio(ctx, packet); err != nil {
		t.Fatalf("SendAudio() error = %v", err)
	}
	taskPayload := server.readPayload(t)
	if !bytes.Equal(taskPayload, []byte("task:7")) {
		t.Fatalf("task payload = %q, want %q", taskPayload, []byte("task:7"))
	}

	gotTask := codec.taskRequests[0]
	if gotTask.Event != EventTaskRequest {
		t.Fatalf("task event = %d, want %d", gotTask.Event, EventTaskRequest)
	}
	if gotTask.Packet.Sequence != 7 || gotTask.Packet.TimestampMS != 560 {
		t.Fatalf("task packet = %+v", gotTask.Packet)
	}
	if !bytes.Equal(gotTask.Packet.PCM, packet.PCM) {
		t.Fatalf("task pcm = %v, want %v", gotTask.Packet.PCM, packet.PCM)
	}

	if err := client.FinishSession(ctx); err != nil {
		t.Fatalf("FinishSession() error = %v", err)
	}
	finishPayload := server.readPayload(t)
	if !bytes.Equal(finishPayload, []byte("finish")) {
		t.Fatalf("finish payload = %q, want %q", finishPayload, []byte("finish"))
	}

	if gotFinish := codec.finishRequests[0]; gotFinish.Event != EventFinishSession {
		t.Fatalf("finish event = %d, want %d", gotFinish.Event, EventFinishSession)
	}
}

func TestClientSendAudioWaitsForSessionStarted(t *testing.T) {
	server := newFakeASTServer(t)
	client, err := NewClient(ClientOptions{
		Endpoint: server.endpoint,
		APIKey:   "api-key",
		Codec:    &recordingCodec{},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close(websocket.StatusNormalClosure, "test done")
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := client.StartSession(ctx, StartSessionParams{
		SessionID:      "session-wait",
		Mode:           ModeS2T,
		SourceLanguage: "en",
		TargetLanguage: "zh",
	}); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	_ = server.readHeaders(t)
	_ = server.readPayload(t)

	sendCtx, sendCancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer sendCancel()
	err = client.SendAudio(sendCtx, audio.Packet{Sequence: 1, PCM: []byte{1, 0}})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("SendAudio() error = %v, want context deadline exceeded", err)
	}
}

func TestClientPublishesDecodedProviderEvents(t *testing.T) {
	server := newFakeASTServer(t)
	client, err := NewClient(ClientOptions{
		Endpoint: server.endpoint,
		APIKey:   "api-key",
		Codec:    &recordingCodec{},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close(websocket.StatusNormalClosure, "test done")
	})

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	if err := client.StartSession(ctx, StartSessionParams{
		SessionID:      "session-events",
		Mode:           ModeS2T,
		SourceLanguage: "en",
		TargetLanguage: "zh",
	}); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	_ = server.readHeaders(t)
	_ = server.readPayload(t)

	server.writeProviderPayload(t, []byte("translation:seg-1:你好"))
	got := readClientEvent(t, client)
	if got.Event != EventTranslationSubtitleResponse || got.SegmentID != "seg-1" || got.Text != "你好" {
		t.Fatalf("provider event = %+v", got)
	}
}

func TestRealClientSatisfiesSessionRunnerContract(t *testing.T) {
	var _ interface {
		StartSession(context.Context, StartSessionParams) error
		SendAudio(context.Context, audio.Packet) error
		FinishSession(context.Context) error
		Events() <-chan ProviderEvent
		Errors() <-chan error
	} = (*Client)(nil)
}

type recordingCodec struct {
	startRequests  []StartSessionRequest
	taskRequests   []TaskRequest
	finishRequests []FinishSessionRequest
}

func (c *recordingCodec) EncodeStartSession(req StartSessionRequest) ([]byte, error) {
	c.startRequests = append(c.startRequests, req)
	return []byte("start:" + req.RequestMeta.SessionID), nil
}

func (c *recordingCodec) EncodeTaskRequest(req TaskRequest) ([]byte, error) {
	c.taskRequests = append(c.taskRequests, req)
	return []byte("task:" + strconv.FormatUint(req.Packet.Sequence, 10)), nil
}

func (c *recordingCodec) EncodeFinishSession(req FinishSessionRequest) ([]byte, error) {
	c.finishRequests = append(c.finishRequests, req)
	return []byte("finish"), nil
}

func (c *recordingCodec) DecodeProviderEvent(payload []byte) (ProviderEvent, error) {
	text := string(payload)
	switch text {
	case "session-started":
		return ProviderEvent{Event: EventSessionStarted}, nil
	case "session-finished":
		return ProviderEvent{Event: EventSessionFinished}, nil
	}
	if strings.HasPrefix(text, "translation:") {
		parts := strings.SplitN(text, ":", 3)
		if len(parts) == 3 {
			return ProviderEvent{
				Event:     EventTranslationSubtitleResponse,
				SegmentID: parts[1],
				Text:      parts[2],
			}, nil
		}
	}
	return ProviderEvent{}, errors.New("unknown test provider payload")
}

type fakeASTServer struct {
	server    *httptest.Server
	endpoint  string
	headers   chan http.Header
	payloads  chan []byte
	responses chan []byte
}

func newFakeASTServer(t *testing.T) *fakeASTServer {
	t.Helper()

	fake := &fakeASTServer{
		headers:   make(chan http.Header, 1),
		payloads:  make(chan []byte, 8),
		responses: make(chan []byte, 8),
	}
	fake.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set(HeaderLogID, "log-123")
		fake.headers <- r.Header.Clone()

		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close(websocket.StatusNormalClosure, "server closed")

		go func() {
			for payload := range fake.responses {
				_ = conn.Write(context.Background(), websocket.MessageBinary, payload)
			}
		}()

		for {
			messageType, payload, err := conn.Read(context.Background())
			if err != nil {
				return
			}
			if messageType == websocket.MessageBinary {
				fake.payloads <- append([]byte(nil), payload...)
			}
		}
	}))
	fake.endpoint = "ws" + strings.TrimPrefix(fake.server.URL, "http")
	t.Cleanup(fake.server.Close)

	return fake
}

func (s *fakeASTServer) writeProviderPayload(t *testing.T, payload []byte) {
	t.Helper()

	select {
	case s.responses <- append([]byte(nil), payload...):
	case <-time.After(time.Second):
		t.Fatal("timed out sending provider payload")
	}
}

func readClientEvent(t *testing.T, client *Client) ProviderEvent {
	t.Helper()

	select {
	case event := <-client.Events():
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for client event")
		return ProviderEvent{}
	}
}

func (s *fakeASTServer) readHeaders(t *testing.T) http.Header {
	t.Helper()

	select {
	case headers := <-s.headers:
		return headers
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket headers")
		return nil
	}
}

func (s *fakeASTServer) readPayload(t *testing.T) []byte {
	t.Helper()

	select {
	case payload := <-s.payloads:
		return payload
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for websocket payload")
		return nil
	}
}
