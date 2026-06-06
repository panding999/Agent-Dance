package httpapi

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"github.com/panding999/agent-dance/backend/internal/doubao/ast"
	"github.com/panding999/agent-dance/backend/internal/live"
	"github.com/panding999/agent-dance/backend/internal/store"
	"github.com/panding999/agent-dance/backend/internal/subtitle"
	"nhooyr.io/websocket"
)

func TestEffectLiveGatewayWithRealDoubaoAST(t *testing.T) {
	if os.Getenv("RUN_DOUBAO_EFFECT") != "1" {
		t.Skip("set RUN_DOUBAO_EFFECT=1 to run the real live gateway effect test")
	}

	options := effectASTOptions(t)
	audioPath := effectAudioPath(t)

	st, err := store.Open(context.Background(), filepath.Join(t.TempDir(), "agent-dance.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() {
		if err := st.Close(); err != nil {
			t.Fatalf("close store: %v", err)
		}
	})

	server := httptest.NewServer(NewServerWithOptions(st, ServerOptions{
		LiveRunnerFactory: func(store.Session) (*live.SessionRunner, error) {
			client, err := ast.NewClient(options)
			if err != nil {
				return nil, err
			}
			return live.NewSessionRunner(client, live.SessionRunnerOptions{})
		},
	}).Handler())
	defer server.Close()

	session := createEffectSession(t, server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()

	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http")+"/api/live/ws?sessionId="+session.ID, nil)
	if err != nil {
		t.Fatalf("dial live websocket: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "effect test done")

	ready := readEffectReady(t, ctx, conn)
	if ready.SessionID != session.ID {
		t.Fatalf("ready session_id = %q, want %q", ready.SessionID, session.ID)
	}

	events := make(chan subtitle.InterpretationEvent, 16)
	readErrs := make(chan error, 1)
	go readEffectEvents(ctx, conn, events, readErrs)

	sendErrs := make(chan error, 1)
	go func() {
		sendErrs <- sendEffectAudio(ctx, conn, effectPCM(t, audioPath))
	}()

	var lastSource subtitle.InterpretationEvent
	for {
		select {
		case <-ctx.Done():
			if lastSource.SourceText != "" {
				t.Fatalf("timed out waiting for translated subtitle text; last source event: type=%s sourceText=%q", lastSource.Type, lastSource.SourceText)
			}
			t.Fatal("timed out waiting for subtitle event")
		case err := <-readErrs:
			t.Fatalf("read live event: %v", err)
		case err := <-sendErrs:
			if err != nil {
				t.Fatalf("send live audio: %v", err)
			}
		case event := <-events:
			if event.Text != "" {
				t.Logf("received translated subtitle event: type=%s text=%q sourceText=%q", event.Type, event.Text, event.SourceText)
				return
			}
			if event.SourceText != "" {
				lastSource = event
			}
		}
	}
}

func effectASTOptions(t *testing.T) ast.ClientOptions {
	t.Helper()

	apiKey := strings.TrimSpace(os.Getenv("DOUBAO_API_KEY"))
	appID := strings.TrimSpace(os.Getenv("DOUBAO_APP_ID"))
	appKey := strings.TrimSpace(os.Getenv("DOUBAO_APP_KEY"))
	accessKey := strings.TrimSpace(os.Getenv("DOUBAO_ACCESS_KEY"))
	if apiKey == "" && appKey == "" && (appID == "" || accessKey == "") {
		t.Skip("missing Doubao credentials for effect test")
	}

	return ast.ClientOptions{
		Endpoint:   strings.TrimSpace(os.Getenv("DOUBAO_AST_ENDPOINT")),
		APIKey:     apiKey,
		AppID:      appID,
		AppKey:     appKey,
		AccessKey:  accessKey,
		ResourceID: strings.TrimSpace(os.Getenv("DOUBAO_AST_RESOURCE_ID")),
		ModelID:    strings.TrimSpace(os.Getenv("DOUBAO_AST_MODEL_ID")),
		Codec:      ast.ProtobufCodec{},
	}
}

func effectAudioPath(t *testing.T) string {
	t.Helper()

	for _, key := range []string{"DOUBAO_AST_EFFECT_AUDIO", "DOUBAO_AST_SMOKE_AUDIO"} {
		path := strings.TrimSpace(os.Getenv(key))
		if path == "" {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s is not readable: %v", key, err)
		}
		return path
	}
	t.Skip("set DOUBAO_AST_EFFECT_AUDIO to a local 16kHz 16-bit mono wav/pcm speech sample")
	return ""
}

func createEffectSession(t *testing.T, serverURL string) store.Session {
	t.Helper()

	body := bytes.NewReader([]byte(`{"mode":"live","source_language":"zh","target_language":"en","voice_enabled":false}`))
	resp, err := http.Post(serverURL+"/api/sessions", "application/json", body)
	if err != nil {
		t.Fatalf("create session request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create session status = %d", resp.StatusCode)
	}

	var session store.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return session
}

func readEffectReady(t *testing.T, ctx context.Context, conn *websocket.Conn) live.Event {
	t.Helper()

	messageType, payload, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read ready event: %v", err)
	}
	if messageType != websocket.MessageText {
		t.Fatalf("ready message type = %v, want text", messageType)
	}

	var event live.Event
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode ready event: %v", err)
	}
	if event.Type != live.EventSessionReady {
		if event.Type == live.EventSessionError {
			t.Fatalf("live session error before ready: %s %s", event.Code, event.Message)
		}
		t.Fatalf("ready event type = %q, want %q", event.Type, live.EventSessionReady)
	}
	return event
}

func readEffectEvents(ctx context.Context, conn *websocket.Conn, events chan<- subtitle.InterpretationEvent, errs chan<- error) {
	for {
		messageType, payload, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() == nil {
				errs <- err
			}
			return
		}
		if messageType != websocket.MessageText {
			continue
		}

		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			errs <- err
			return
		}
		switch envelope.Type {
		case live.EventAudioFrameAccepted:
			continue
		case live.EventSessionError:
			var event live.Event
			_ = json.Unmarshal(payload, &event)
			errs <- errors.New(strings.TrimSpace(event.Code + " " + event.Message))
			return
		case string(subtitle.EventSegmentPartial), string(subtitle.EventSegmentFinal):
			var event subtitle.InterpretationEvent
			if err := json.Unmarshal(payload, &event); err != nil {
				errs <- err
				return
			}
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
	}
}

func sendEffectAudio(ctx context.Context, conn *websocket.Conn, pcm []byte) error {
	maxBytes := audio.DoubaoPacketBytes * 150
	if len(pcm) > maxBytes {
		pcm = pcm[:maxBytes]
	}

	var sequence uint32 = 1
	var timestampMS uint64
	for len(pcm) > 0 {
		size := audio.DoubaoPacketBytes
		if len(pcm) < size {
			size = len(pcm)
		}
		frame := makeEffectBrowserFrame(sequence, timestampMS, pcm[:size])
		if err := conn.Write(ctx, websocket.MessageBinary, frame); err != nil {
			return err
		}
		pcm = pcm[size:]
		sequence++
		timestampMS += audio.DoubaoPacketDurationMS

		timer := time.NewTimer(time.Duration(audio.DoubaoPacketDurationMS) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	return nil
}

func effectPCM(t *testing.T, path string) []byte {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read audio: %v", err)
	}
	if len(content) >= 12 && string(content[:4]) == "RIFF" {
		return effectWAVData(t, content)
	}
	return content
}

func effectWAVData(t *testing.T, content []byte) []byte {
	t.Helper()

	offset := 12
	for offset+8 <= len(content) {
		chunkID := string(content[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(content[offset+4 : offset+8]))
		dataStart := offset + 8
		dataEnd := dataStart + chunkSize
		if dataEnd > len(content) {
			t.Fatal("invalid wav chunk size")
		}
		if chunkID == "data" {
			return content[dataStart:dataEnd]
		}
		offset = dataEnd
		if offset%2 == 1 {
			offset++
		}
	}
	t.Fatal("wav data chunk not found")
	return nil
}

func makeEffectBrowserFrame(sequence uint32, timestampMS uint64, pcm []byte) []byte {
	raw := make([]byte, audio.BrowserFrameHeaderSize+len(pcm))
	binary.LittleEndian.PutUint32(raw[0:4], sequence)
	binary.LittleEndian.PutUint64(raw[4:12], timestampMS)
	copy(raw[12:], pcm)
	return raw
}
