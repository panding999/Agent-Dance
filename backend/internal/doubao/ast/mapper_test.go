package ast

import (
	"bytes"
	"testing"

	"github.com/panding999/agent-dance/backend/internal/subtitle"
)

func TestMapProviderEventMapsSourceSubtitleResponseToPartialSourceText(t *testing.T) {
	got, ok := MapProviderEvent(ProviderEvent{
		Event:       EventSourceSubtitleResponse,
		SegmentID:   "seg-1",
		Text:        "hello",
		StartTimeMS: 100,
		EndTimeMS:   360,
	})
	if !ok {
		t.Fatal("MapProviderEvent() ok = false, want true")
	}

	if got.Type != subtitle.EventSegmentPartial {
		t.Fatalf("type = %q, want %q", got.Type, subtitle.EventSegmentPartial)
	}
	if got.SegmentID != "seg-1" {
		t.Fatalf("segment_id = %q, want seg-1", got.SegmentID)
	}
	if got.SourceText != "hello" {
		t.Fatalf("source_text = %q, want hello", got.SourceText)
	}
	if got.Text != "" {
		t.Fatalf("text = %q, want empty target text", got.Text)
	}
	if got.StartMS != 100 || got.EndMS != 360 {
		t.Fatalf("time range = %d-%d, want 100-360", got.StartMS, got.EndMS)
	}
}

func TestMapProviderEventMapsSourceSubtitleEndToFinalSourceMetadata(t *testing.T) {
	got, ok := MapProviderEvent(ProviderEvent{
		Event:       EventSourceSubtitleEnd,
		SegmentID:   "seg-1",
		Text:        "hello world",
		StartTimeMS: 100,
		EndTimeMS:   700,
	})
	if !ok {
		t.Fatal("MapProviderEvent() ok = false, want true")
	}

	if got.Type != subtitle.EventSegmentFinal {
		t.Fatalf("type = %q, want %q", got.Type, subtitle.EventSegmentFinal)
	}
	if got.SourceText != "hello world" {
		t.Fatalf("source_text = %q, want hello world", got.SourceText)
	}
	if got.Text != "" {
		t.Fatalf("text = %q, want empty target text", got.Text)
	}
	if got.StartMS != 100 || got.EndMS != 700 {
		t.Fatalf("time range = %d-%d, want 100-700", got.StartMS, got.EndMS)
	}
}

func TestMapProviderEventMapsTranslationSubtitleResponseToPartialTargetText(t *testing.T) {
	got, ok := MapProviderEvent(ProviderEvent{
		Event:       EventTranslationSubtitleResponse,
		SegmentID:   "seg-2",
		Text:        "你好",
		StartTimeMS: 800,
		EndTimeMS:   1000,
	})
	if !ok {
		t.Fatal("MapProviderEvent() ok = false, want true")
	}

	if got.Type != subtitle.EventSegmentPartial {
		t.Fatalf("type = %q, want %q", got.Type, subtitle.EventSegmentPartial)
	}
	if got.Text != "你好" {
		t.Fatalf("text = %q, want 你好", got.Text)
	}
	if got.SourceText != "" {
		t.Fatalf("source_text = %q, want empty source text", got.SourceText)
	}
}

func TestMapProviderEventMapsTranslationSubtitleEndToFinalTargetText(t *testing.T) {
	got, ok := MapProviderEvent(ProviderEvent{
		Event:       EventTranslationSubtitleEnd,
		SegmentID:   "seg-2",
		Text:        "你好，世界",
		StartTimeMS: 800,
		EndTimeMS:   1300,
	})
	if !ok {
		t.Fatal("MapProviderEvent() ok = false, want true")
	}

	if got.Type != subtitle.EventSegmentFinal {
		t.Fatalf("type = %q, want %q", got.Type, subtitle.EventSegmentFinal)
	}
	if got.Text != "你好，世界" {
		t.Fatalf("text = %q, want 你好，世界", got.Text)
	}
	if got.StartMS != 800 || got.EndMS != 1300 {
		t.Fatalf("time range = %d-%d, want 800-1300", got.StartMS, got.EndMS)
	}
}

func TestMapProviderEventMapsTTSResponseToAudioDelta(t *testing.T) {
	audioBytes := []byte{1, 0, 2, 0}
	got, ok := MapProviderEvent(ProviderEvent{
		Event:      EventTTSResponse,
		SegmentID:  "seg-3",
		Data:       audioBytes,
		AudioCodec: string(subtitle.CodecPCM),
	})
	if !ok {
		t.Fatal("MapProviderEvent() ok = false, want true")
	}

	if got.Type != subtitle.EventAudioDelta {
		t.Fatalf("type = %q, want %q", got.Type, subtitle.EventAudioDelta)
	}
	if got.SegmentID != "seg-3" {
		t.Fatalf("segment_id = %q, want seg-3", got.SegmentID)
	}
	if got.Codec != subtitle.CodecPCM {
		t.Fatalf("codec = %q, want %q", got.Codec, subtitle.CodecPCM)
	}
	if !bytes.Equal(got.Audio, audioBytes) {
		t.Fatalf("audio = %v, want %v", got.Audio, audioBytes)
	}

	audioBytes[0] = 99
	if got.Audio[0] == 99 {
		t.Fatal("mapped audio delta shares mutable provider payload")
	}
}

func TestMapProviderEventMapsSessionFailedToSessionError(t *testing.T) {
	got, ok := MapProviderEvent(ProviderEvent{
		Event: EventSessionFailed,
		Error: &ProviderError{
			Code:    "provider_timeout",
			Message: "provider timed out",
			LogID:   "log-123",
		},
	})
	if !ok {
		t.Fatal("MapProviderEvent() ok = false, want true")
	}

	if got.Type != subtitle.EventSessionError {
		t.Fatalf("type = %q, want %q", got.Type, subtitle.EventSessionError)
	}
	if got.Code != "provider_timeout" || got.Message != "provider timed out" || got.ProviderLogID != "log-123" {
		t.Fatalf("session error = %+v", got)
	}
}

func TestEventNormalizerAccumulatesIncrementalTranslationPartials(t *testing.T) {
	normalizer := NewEventNormalizer(4)

	first, ok := normalizer.Map(ProviderEvent{
		Event:       EventTranslationSubtitleResponse,
		SegmentID:   "seg-1",
		Text:        "我很好",
		StartTimeMS: 100,
		EndTimeMS:   800,
	})
	if !ok {
		t.Fatal("first Map() ok = false, want true")
	}
	if first.Text != "我很好" {
		t.Fatalf("first text = %q, want 我很好", first.Text)
	}

	second, ok := normalizer.Map(ProviderEvent{
		Event:       EventTranslationSubtitleResponse,
		SegmentID:   "seg-1",
		Text:        "。",
		StartTimeMS: 100,
		EndTimeMS:   900,
	})
	if !ok {
		t.Fatal("second Map() ok = false, want true")
	}
	if second.Text != "我很好。" {
		t.Fatalf("second text = %q, want 我很好。", second.Text)
	}
	if second.SourceText != "" {
		t.Fatalf("second source_text = %q, want empty", second.SourceText)
	}
}

func TestEventNormalizerUsesCompleteEndTextWithoutDuplicatingPartials(t *testing.T) {
	normalizer := NewEventNormalizer(4)

	_, _ = normalizer.Map(ProviderEvent{
		Event:     EventSourceSubtitleResponse,
		SegmentID: "seg-2",
		Text:      "I'm",
	})
	_, _ = normalizer.Map(ProviderEvent{
		Event:     EventSourceSubtitleResponse,
		SegmentID: "seg-2",
		Text:      " fine",
	})
	got, ok := normalizer.Map(ProviderEvent{
		Event:       EventSourceSubtitleEnd,
		SegmentID:   "seg-2",
		Text:        "I'm fine.",
		StartTimeMS: 120,
		EndTimeMS:   920,
	})
	if !ok {
		t.Fatal("Map() ok = false, want true")
	}
	if got.Type != subtitle.EventSegmentFinal {
		t.Fatalf("type = %q, want %q", got.Type, subtitle.EventSegmentFinal)
	}
	if got.SourceText != "I'm fine." {
		t.Fatalf("source_text = %q, want I'm fine.", got.SourceText)
	}
	if got.Text != "" {
		t.Fatalf("text = %q, want empty", got.Text)
	}
}

func TestEventNormalizerStoresProviderEventSummaries(t *testing.T) {
	normalizer := NewEventNormalizer(2)

	_, _ = normalizer.Map(ProviderEvent{Event: EventSourceSubtitleResponse, SegmentID: "seg-1", Text: "hello"})
	_, _ = normalizer.Map(ProviderEvent{Event: EventTranslationSubtitleResponse, SegmentID: "seg-1", Text: "你好"})
	_, _ = normalizer.Map(ProviderEvent{Event: EventSessionFailed, Error: &ProviderError{Code: "bad", LogID: "log-9"}})

	summaries := normalizer.Summaries()
	if len(summaries) != 2 {
		t.Fatalf("len(summaries) = %d, want 2", len(summaries))
	}
	if summaries[0].Event != EventTranslationSubtitleResponse || summaries[0].SegmentID != "seg-1" {
		t.Fatalf("summary[0] = %+v", summaries[0])
	}
	if summaries[0].TextBytes == 0 {
		t.Fatalf("summary[0].TextBytes = %d, want non-zero", summaries[0].TextBytes)
	}
	if summaries[1].Event != EventSessionFailed || summaries[1].ErrorCode != "bad" || summaries[1].ProviderLogID != "log-9" {
		t.Fatalf("summary[1] = %+v", summaries[1])
	}

	summaries[0].SegmentID = "mutated"
	if normalizer.Summaries()[0].SegmentID == "mutated" {
		t.Fatal("Summaries() exposes mutable internal summary slice")
	}
}

func TestMapProviderEventIgnoresUnmappedProviderEvents(t *testing.T) {
	got, ok := MapProviderEvent(ProviderEvent{Event: EventUsageResponse})
	if ok {
		t.Fatalf("MapProviderEvent() ok = true with event %+v", got)
	}
}
