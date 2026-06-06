package live

import (
	"context"
	"errors"
	"sync"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"github.com/panding999/agent-dance/backend/internal/doubao/ast"
	"github.com/panding999/agent-dance/backend/internal/store"
	"github.com/panding999/agent-dance/backend/internal/subtitle"
)

var ErrSessionRunnerNotStarted = errors.New("session runner is not started")

type ASTSessionClient interface {
	StartSession(context.Context, ast.StartSessionParams) error
	SendAudio(context.Context, audio.Packet) error
	FinishSession(context.Context) error
	Events() <-chan ast.ProviderEvent
	Errors() <-chan error
}

type SessionRunnerOptions struct {
	PCMFormat  audio.PCMFormat
	Normalizer *ast.EventNormalizer
	Stabilizer *subtitle.Stabilizer
}

type SessionRunner struct {
	astClient  ASTSessionClient
	options    SessionRunnerOptions
	normalizer *ast.EventNormalizer
	stabilizer *subtitle.Stabilizer

	mu         sync.Mutex
	packetizer *audio.Packetizer
	started    bool
	cancel     context.CancelFunc

	events chan subtitle.InterpretationEvent
	errors chan error
}

func NewSessionRunner(astClient ASTSessionClient, options SessionRunnerOptions) (*SessionRunner, error) {
	if options.PCMFormat == (audio.PCMFormat{}) {
		options.PCMFormat = audio.DefaultPCMFormat
	}
	if options.Normalizer == nil {
		options.Normalizer = ast.NewEventNormalizer(128)
	}
	if options.Stabilizer == nil {
		options.Stabilizer = subtitle.NewStabilizer(subtitle.StabilizerOptions{})
	}

	packetizer, err := audio.NewPacketizer(options.PCMFormat)
	if err != nil {
		return nil, err
	}

	return &SessionRunner{
		astClient:  astClient,
		options:    options,
		normalizer: options.Normalizer,
		stabilizer: options.Stabilizer,
		packetizer: packetizer,
		events:     make(chan subtitle.InterpretationEvent, 32),
		errors:     make(chan error, 1),
	}, nil
}

func (r *SessionRunner) Start(ctx context.Context, session store.Session) error {
	params := ast.StartSessionParams{
		SessionID:      session.ID,
		Mode:           ast.ModeS2T,
		SourceLanguage: session.SourceLanguage,
		TargetLanguage: session.TargetLanguage,
	}
	if session.VoiceEnabled {
		params.Mode = ast.ModeS2S
	}
	if err := r.astClient.StartSession(ctx, params); err != nil {
		return err
	}

	runCtx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.started = true
	r.cancel = cancel
	r.mu.Unlock()

	go r.forwardProviderEvents(runCtx)
	return nil
}

func (r *SessionRunner) PushAudioFrame(ctx context.Context, frame audio.PCMFrame) error {
	r.mu.Lock()
	started := r.started
	packetizer := r.packetizer
	r.mu.Unlock()

	if !started {
		return ErrSessionRunnerNotStarted
	}

	packets, err := packetizer.Push(frame)
	if err != nil {
		return err
	}
	for _, packet := range packets {
		if err := r.astClient.SendAudio(ctx, packet); err != nil {
			return err
		}
	}
	return nil
}

func (r *SessionRunner) Finish(ctx context.Context) error {
	r.mu.Lock()
	started := r.started
	cancel := r.cancel
	r.started = false
	r.cancel = nil
	r.mu.Unlock()

	if cancel != nil {
		cancel()
	}
	if !started {
		return nil
	}
	return r.astClient.FinishSession(ctx)
}

func (r *SessionRunner) Events() <-chan subtitle.InterpretationEvent {
	return r.events
}

func (r *SessionRunner) Errors() <-chan error {
	return r.errors
}

func (r *SessionRunner) forwardProviderEvents(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-r.astClient.Errors():
			if ok && err != nil {
				r.sendError(ctx, err)
			}
			return
		case providerEvent, ok := <-r.astClient.Events():
			if !ok {
				return
			}
			normalized, ok := r.normalizer.Map(providerEvent)
			if !ok {
				continue
			}
			for _, event := range r.stabilizer.Apply(normalized) {
				if !r.sendEvent(ctx, event) {
					return
				}
				if event.Type == subtitle.EventSessionError {
					return
				}
			}
		}
	}
}

func (r *SessionRunner) sendEvent(ctx context.Context, event subtitle.InterpretationEvent) bool {
	select {
	case <-ctx.Done():
		return false
	case r.events <- event:
		return true
	}
}

func (r *SessionRunner) sendError(ctx context.Context, err error) {
	select {
	case <-ctx.Done():
	case r.errors <- err:
	default:
	}
}
