package live

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"github.com/panding999/agent-dance/backend/internal/doubao/ast"
	"github.com/panding999/agent-dance/backend/internal/store"
	"github.com/panding999/agent-dance/backend/internal/subtitle"
)

func TestSessionRunnerStartsASTAndForwardsPacketizedAudio(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	fakeAST := newFakeASTClient()
	runner, err := NewSessionRunner(fakeAST, SessionRunnerOptions{})
	if err != nil {
		t.Fatalf("NewSessionRunner() error = %v", err)
	}

	session := store.Session{
		ID:             "session-1",
		SourceLanguage: "en",
		TargetLanguage: "zh",
	}
	if err := runner.Start(ctx, session); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	started := fakeAST.startParams()
	if started.SessionID != "session-1" {
		t.Fatalf("started session id = %q, want session-1", started.SessionID)
	}
	if started.Mode != ast.ModeS2T {
		t.Fatalf("started mode = %q, want %q", started.Mode, ast.ModeS2T)
	}
	if started.SourceLanguage != "en" || started.TargetLanguage != "zh" {
		t.Fatalf("started language pair = %q -> %q, want en -> zh", started.SourceLanguage, started.TargetLanguage)
	}

	if err := runner.PushAudioFrame(ctx, audio.PCMFrame{
		Sequence:    1,
		TimestampMS: 2000,
		PCM:         makeRunnerPCM(audio.DoubaoPacketBytes),
	}); err != nil {
		t.Fatalf("PushAudioFrame() error = %v", err)
	}

	packets := fakeAST.sentPackets()
	if len(packets) != 1 {
		t.Fatalf("len(packets) = %d, want 1", len(packets))
	}
	if packets[0].Sequence != 0 || packets[0].TimestampMS != 2000 {
		t.Fatalf("packet = %+v, want sequence 0 timestamp 2000", packets[0])
	}

	if err := runner.Finish(ctx); err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if !fakeAST.isFinished() {
		t.Fatal("fake AST was not finished")
	}
}

func TestSessionRunnerUsesS2SModeWhenVoiceEnabled(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	fakeAST := newFakeASTClient()
	runner, err := NewSessionRunner(fakeAST, SessionRunnerOptions{})
	if err != nil {
		t.Fatalf("NewSessionRunner() error = %v", err)
	}

	if err := runner.Start(ctx, store.Session{
		ID:             "session-voice",
		SourceLanguage: "en",
		TargetLanguage: "zh",
		VoiceEnabled:   true,
	}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if fakeAST.startParams().Mode != ast.ModeS2S {
		t.Fatalf("mode = %q, want %q", fakeAST.startParams().Mode, ast.ModeS2S)
	}
}

func TestSessionRunnerNormalizesAndStabilizesProviderEvents(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	fakeAST := newFakeASTClient()
	runner, err := NewSessionRunner(fakeAST, SessionRunnerOptions{})
	if err != nil {
		t.Fatalf("NewSessionRunner() error = %v", err)
	}
	if err := runner.Start(ctx, store.Session{ID: "session-2", SourceLanguage: "en", TargetLanguage: "zh"}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	fakeAST.emit(ast.ProviderEvent{
		Event:       ast.EventTranslationSubtitleResponse,
		SegmentID:   "seg-1",
		Text:        "你好",
		StartTimeMS: 100,
		EndTimeMS:   300,
	})
	got := readRunnerEvent(t, runner)
	if got.Type != subtitle.EventSegmentPartial || got.Text != "你好" {
		t.Fatalf("subtitle event = %+v", got)
	}

	fakeAST.emit(ast.ProviderEvent{
		Event:      ast.EventTTSResponse,
		SegmentID:  "seg-1",
		Data:       []byte{1, 0, 2, 0},
		AudioCodec: string(subtitle.CodecPCM),
	})
	audioEvent := readRunnerEvent(t, runner)
	if audioEvent.Type != subtitle.EventAudioDelta || audioEvent.Codec != subtitle.CodecPCM || len(audioEvent.Audio) != 4 {
		t.Fatalf("audio event = %+v", audioEvent)
	}
}

func TestSessionRunnerReportsASTFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	fakeAST := newFakeASTClient()
	runner, err := NewSessionRunner(fakeAST, SessionRunnerOptions{})
	if err != nil {
		t.Fatalf("NewSessionRunner() error = %v", err)
	}
	if err := runner.Start(ctx, store.Session{ID: "session-3", SourceLanguage: "en", TargetLanguage: "zh"}); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	fakeAST.fail(errors.New("ast dropped"))
	select {
	case err := <-runner.Errors():
		if err == nil || err.Error() != "ast dropped" {
			t.Fatalf("runner error = %v, want ast dropped", err)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner error")
	}
}

func readRunnerEvent(t *testing.T, runner *SessionRunner) subtitle.InterpretationEvent {
	t.Helper()

	select {
	case event := <-runner.Events():
		return event
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for runner event")
		return subtitle.InterpretationEvent{}
	}
}

type fakeASTClient struct {
	events chan ast.ProviderEvent
	errors chan error

	mu       sync.Mutex
	started  ast.StartSessionParams
	packets  []audio.Packet
	finished bool
}

func newFakeASTClient() *fakeASTClient {
	return &fakeASTClient{
		events: make(chan ast.ProviderEvent, 8),
		errors: make(chan error, 1),
	}
}

func (f *fakeASTClient) StartSession(_ context.Context, params ast.StartSessionParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.started = params
	return nil
}

func (f *fakeASTClient) SendAudio(_ context.Context, packet audio.Packet) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	packet.PCM = append([]byte(nil), packet.PCM...)
	f.packets = append(f.packets, packet)
	return nil
}

func (f *fakeASTClient) FinishSession(context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished = true
	return nil
}

func (f *fakeASTClient) Events() <-chan ast.ProviderEvent {
	return f.events
}

func (f *fakeASTClient) Errors() <-chan error {
	return f.errors
}

func (f *fakeASTClient) emit(event ast.ProviderEvent) {
	f.events <- event
}

func (f *fakeASTClient) fail(err error) {
	f.errors <- err
}

func (f *fakeASTClient) startParams() ast.StartSessionParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.started
}

func (f *fakeASTClient) sentPackets() []audio.Packet {
	f.mu.Lock()
	defer f.mu.Unlock()
	packets := make([]audio.Packet, len(f.packets))
	copy(packets, f.packets)
	return packets
}

func (f *fakeASTClient) isFinished() bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.finished
}

func makeRunnerPCM(size int) []byte {
	pcm := make([]byte, size)
	for i := range pcm {
		pcm[i] = byte(i % 251)
	}
	return pcm
}
