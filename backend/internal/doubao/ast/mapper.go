package ast

import (
	"strings"

	"github.com/panding999/agent-dance/backend/internal/subtitle"
)

const defaultProviderErrorCode = "provider_error"

type ProviderEventSummary struct {
	Event         EventType
	SegmentID     string
	StartTimeMS   int64
	EndTimeMS     int64
	TextBytes     int
	DataBytes     int
	ErrorCode     string
	ProviderLogID string
}

type EventNormalizer struct {
	maxSummaries int
	summaries    []ProviderEventSummary
}

func NewEventNormalizer(maxSummaries int) *EventNormalizer {
	if maxSummaries <= 0 {
		maxSummaries = 128
	}
	return &EventNormalizer{
		maxSummaries: maxSummaries,
		summaries:    make([]ProviderEventSummary, 0, maxSummaries),
	}
}

func (n *EventNormalizer) Map(event ProviderEvent) (subtitle.InterpretationEvent, bool) {
	n.record(event.Summary())
	return MapProviderEvent(event)
}

func (n *EventNormalizer) Summaries() []ProviderEventSummary {
	summaries := make([]ProviderEventSummary, len(n.summaries))
	copy(summaries, n.summaries)
	return summaries
}

func (n *EventNormalizer) record(summary ProviderEventSummary) {
	if len(n.summaries) == n.maxSummaries {
		copy(n.summaries, n.summaries[1:])
		n.summaries[len(n.summaries)-1] = summary
		return
	}
	n.summaries = append(n.summaries, summary)
}

func MapProviderEvent(event ProviderEvent) (subtitle.InterpretationEvent, bool) {
	switch event.Event {
	case EventSourceSubtitleResponse:
		return segmentEvent(subtitle.EventSegmentPartial, event.SegmentID, "", event.Text, event.StartTimeMS, event.EndTimeMS), true
	case EventSourceSubtitleEnd:
		return segmentEvent(subtitle.EventSegmentFinal, event.SegmentID, "", event.Text, event.StartTimeMS, event.EndTimeMS), true
	case EventTranslationSubtitleResponse:
		return segmentEvent(subtitle.EventSegmentPartial, event.SegmentID, event.Text, "", event.StartTimeMS, event.EndTimeMS), true
	case EventTranslationSubtitleEnd:
		return segmentEvent(subtitle.EventSegmentFinal, event.SegmentID, event.Text, "", event.StartTimeMS, event.EndTimeMS), true
	case EventTTSResponse:
		codec := subtitle.AudioCodec(strings.TrimSpace(event.AudioCodec))
		if codec == "" {
			codec = subtitle.CodecPCM
		}
		return subtitle.NewAudioDelta(event.SegmentID, event.Data, codec), true
	case EventSessionFailed:
		return sessionErrorEvent(event), true
	default:
		return subtitle.InterpretationEvent{}, false
	}
}

func segmentEvent(eventType subtitle.EventType, segmentID string, text string, sourceText string, startMS int64, endMS int64) subtitle.InterpretationEvent {
	return subtitle.InterpretationEvent{
		Type:       eventType,
		SegmentID:  segmentID,
		Text:       text,
		SourceText: sourceText,
		StartMS:    startMS,
		EndMS:      endMS,
	}
}

func sessionErrorEvent(event ProviderEvent) subtitle.InterpretationEvent {
	code := defaultProviderErrorCode
	message := strings.TrimSpace(event.Text)
	logID := ""

	if event.Error != nil {
		if strings.TrimSpace(event.Error.Code) != "" {
			code = strings.TrimSpace(event.Error.Code)
		}
		message = strings.TrimSpace(event.Error.Message)
		logID = strings.TrimSpace(event.Error.LogID)
	}
	if message == "" {
		message = "provider session failed"
	}

	return subtitle.InterpretationEvent{
		Type:          subtitle.EventSessionError,
		Code:          code,
		Message:       message,
		ProviderLogID: logID,
	}
}

func (event ProviderEvent) Summary() ProviderEventSummary {
	summary := ProviderEventSummary{
		Event:       event.Event,
		SegmentID:   event.SegmentID,
		StartTimeMS: event.StartTimeMS,
		EndTimeMS:   event.EndTimeMS,
		TextBytes:   len(event.Text),
		DataBytes:   len(event.Data),
	}
	if event.Error != nil {
		summary.ErrorCode = strings.TrimSpace(event.Error.Code)
		summary.ProviderLogID = strings.TrimSpace(event.Error.LogID)
	}
	return summary
}
