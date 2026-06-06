package ast

import (
	"errors"
	"strings"

	"github.com/panding999/agent-dance/backend/internal/audio"
)

type EventType int32

const (
	EventStartSession  EventType = 100
	EventFinishSession EventType = 102
	EventTaskRequest   EventType = 200
	EventUpdateConfig  EventType = 201

	EventSessionStarted              EventType = 150
	EventSessionFinished             EventType = 152
	EventSessionFailed               EventType = 153
	EventUsageResponse               EventType = 154
	EventAudioMuted                  EventType = 250
	EventTTSSentenceStart            EventType = 350
	EventTTSSentenceEnd              EventType = 351
	EventTTSResponse                 EventType = 352
	EventSourceSubtitleStart         EventType = 650
	EventSourceSubtitleResponse      EventType = 651
	EventSourceSubtitleEnd           EventType = 652
	EventTranslationSubtitleStart    EventType = 653
	EventTranslationSubtitleResponse EventType = 654
	EventTranslationSubtitleEnd      EventType = 655
)

type SessionMode string

const (
	ModeS2T SessionMode = "s2t"
	ModeS2S SessionMode = "s2s"
)

var (
	ErrMissingSessionID      = errors.New("missing ast session id")
	ErrMissingMode           = errors.New("missing ast session mode")
	ErrUnsupportedMode       = errors.New("unsupported ast session mode")
	ErrMissingSourceLanguage = errors.New("missing ast source language")
	ErrMissingTargetLanguage = errors.New("missing ast target language")
)

type Codec interface {
	EncodeStartSession(StartSessionRequest) ([]byte, error)
	EncodeTaskRequest(TaskRequest) ([]byte, error)
	EncodeFinishSession(FinishSessionRequest) ([]byte, error)
	DecodeProviderEvent([]byte) (ProviderEvent, error)
}

type StartSessionParams struct {
	SessionID      string
	ModelID        string
	Mode           SessionMode
	SourceLanguage string
	TargetLanguage string
	SpeakerID      string
	SpeechRate     float64
	User           UserConfig
	Corpus         Corpus
	TargetAudio    *AudioConfig
}

type StartSessionRequest struct {
	RequestMeta RequestMeta
	Event       EventType
	User        UserConfig
	SourceAudio AudioConfig
	TargetAudio *AudioConfig
	Request     SessionRequest
}

type RequestMeta struct {
	SessionID string
	ModelID   string
}

type UserConfig struct {
	UID        string
	DID        string
	Platform   string
	SDKVersion string
}

type SessionRequest struct {
	Mode           SessionMode
	SpeakerID      string
	SpeechRate     float64
	SourceLanguage string
	TargetLanguage string
	Corpus         Corpus
}

type Corpus struct {
	HotWordsList          []string
	BoostingTableID       string
	BoostingTableName     string
	CorrectWords          map[string]string
	RegexCorrectTableID   string
	RegexCorrectTableName string
	GlossaryList          map[string]string
	GlossaryTableID       string
	GlossaryTableName     string
}

type AudioConfig struct {
	Format  string
	Codec   string
	Rate    int
	Bits    int
	Channel int
}

type TaskRequest struct {
	Event  EventType
	Packet audio.Packet
}

type FinishSessionRequest struct {
	Event     EventType
	SessionID string
}

func DefaultSourceAudioConfig() AudioConfig {
	return AudioConfig{
		Format:  "wav",
		Codec:   "raw",
		Rate:    audio.RequiredSampleRateHz,
		Bits:    audio.RequiredBitsPerSample,
		Channel: audio.RequiredChannels,
	}
}

func DefaultTargetAudioConfig() AudioConfig {
	return AudioConfig{
		Format: "pcm",
		Rate:   24000,
	}
}

func newStartSessionRequest(params StartSessionParams) (StartSessionRequest, error) {
	if err := validateStartSessionParams(params); err != nil {
		return StartSessionRequest{}, err
	}

	var targetAudio *AudioConfig
	if params.TargetAudio != nil {
		copy := *params.TargetAudio
		targetAudio = &copy
	} else if params.Mode == ModeS2S {
		defaultTarget := DefaultTargetAudioConfig()
		targetAudio = &defaultTarget
	}

	return StartSessionRequest{
		RequestMeta: RequestMeta{
			SessionID: strings.TrimSpace(params.SessionID),
			ModelID:   strings.TrimSpace(params.ModelID),
		},
		Event:       EventStartSession,
		User:        params.User,
		SourceAudio: DefaultSourceAudioConfig(),
		TargetAudio: targetAudio,
		Request: SessionRequest{
			Mode:           params.Mode,
			SpeakerID:      strings.TrimSpace(params.SpeakerID),
			SpeechRate:     params.SpeechRate,
			SourceLanguage: strings.TrimSpace(params.SourceLanguage),
			TargetLanguage: strings.TrimSpace(params.TargetLanguage),
			Corpus:         cloneCorpus(params.Corpus),
		},
	}, nil
}

func newTaskRequest(packet audio.Packet) TaskRequest {
	packet.PCM = cloneBytes(packet.PCM)
	return TaskRequest{
		Event:  EventTaskRequest,
		Packet: packet,
	}
}

func newFinishSessionRequest(sessionID string) FinishSessionRequest {
	return FinishSessionRequest{
		Event:     EventFinishSession,
		SessionID: strings.TrimSpace(sessionID),
	}
}

func validateStartSessionParams(params StartSessionParams) error {
	if strings.TrimSpace(params.SessionID) == "" {
		return ErrMissingSessionID
	}
	switch params.Mode {
	case "":
		return ErrMissingMode
	case ModeS2T, ModeS2S:
	default:
		return ErrUnsupportedMode
	}
	if strings.TrimSpace(params.SourceLanguage) == "" {
		return ErrMissingSourceLanguage
	}
	if strings.TrimSpace(params.TargetLanguage) == "" {
		return ErrMissingTargetLanguage
	}
	return nil
}

func cloneCorpus(corpus Corpus) Corpus {
	return Corpus{
		HotWordsList:          cloneStrings(corpus.HotWordsList),
		BoostingTableID:       strings.TrimSpace(corpus.BoostingTableID),
		BoostingTableName:     strings.TrimSpace(corpus.BoostingTableName),
		CorrectWords:          cloneStringMap(corpus.CorrectWords),
		RegexCorrectTableID:   strings.TrimSpace(corpus.RegexCorrectTableID),
		RegexCorrectTableName: strings.TrimSpace(corpus.RegexCorrectTableName),
		GlossaryList:          cloneStringMap(corpus.GlossaryList),
		GlossaryTableID:       strings.TrimSpace(corpus.GlossaryTableID),
		GlossaryTableName:     strings.TrimSpace(corpus.GlossaryTableName),
	}
}

func cloneStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	copyValues := make([]string, len(values))
	copy(copyValues, values)
	return copyValues
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	copyValues := make(map[string]string, len(values))
	for key, value := range values {
		copyValues[key] = value
	}
	return copyValues
}

func cloneBytes(values []byte) []byte {
	if len(values) == 0 {
		return nil
	}
	copyValues := make([]byte, len(values))
	copy(copyValues, values)
	return copyValues
}
