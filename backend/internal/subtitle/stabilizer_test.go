package subtitle

import "testing"

func TestStabilizerUpdatesCurrentLineForPartialsWithoutHistory(t *testing.T) {
	stabilizer := NewStabilizer(StabilizerOptions{})

	first := stabilizer.Apply(InterpretationEvent{
		Type:      EventSegmentPartial,
		SegmentID: "seg-1",
		Text:      "你好",
		StartMS:   100,
		EndMS:     300,
	})
	if len(first) != 1 {
		t.Fatalf("len(first emitted) = %d, want 1", len(first))
	}

	second := stabilizer.Apply(InterpretationEvent{
		Type:      EventSegmentPartial,
		SegmentID: "seg-1",
		Text:      "你好世界",
		StartMS:   100,
		EndMS:     500,
	})
	if len(second) != 1 {
		t.Fatalf("len(second emitted) = %d, want 1", len(second))
	}

	current := stabilizer.Current()
	if len(current) != 1 {
		t.Fatalf("len(current) = %d, want 1", len(current))
	}
	if current[0].Text != "你好世界" {
		t.Fatalf("current text = %q, want 你好世界", current[0].Text)
	}
	if len(stabilizer.Segments()) != 0 {
		t.Fatalf("history rows = %d, want 0 for partials", len(stabilizer.Segments()))
	}
}

func TestStabilizerMergesSourceAndTranslationBySegmentID(t *testing.T) {
	stabilizer := NewStabilizer(StabilizerOptions{})

	stabilizer.Apply(InterpretationEvent{
		Type:       EventSegmentPartial,
		SegmentID:  "seg-1",
		SourceText: "hello",
		StartMS:    100,
		EndMS:      300,
	})
	got := stabilizer.Apply(InterpretationEvent{
		Type:      EventSegmentPartial,
		SegmentID: "seg-1",
		Text:      "你好",
		StartMS:   100,
		EndMS:     320,
	})

	if len(got) != 1 {
		t.Fatalf("len(emitted) = %d, want 1", len(got))
	}
	if got[0].SegmentID != "seg-1" {
		t.Fatalf("emitted segment id = %q, want seg-1", got[0].SegmentID)
	}
	current := stabilizer.Current()
	if current[0].SourceText != "hello" || current[0].Text != "你好" {
		t.Fatalf("current segment = %+v", current[0])
	}
}

func TestStabilizerMergesSourceAndTranslationByTimeWindow(t *testing.T) {
	stabilizer := NewStabilizer(StabilizerOptions{MergeWindowMS: 250})

	stabilizer.Apply(InterpretationEvent{
		Type:       EventSegmentPartial,
		SegmentID:  "source-1",
		SourceText: "hello",
		StartMS:    1000,
		EndMS:      1300,
	})
	got := stabilizer.Apply(InterpretationEvent{
		Type:      EventSegmentPartial,
		SegmentID: "translation-9",
		Text:      "你好",
		StartMS:   1120,
		EndMS:     1380,
	})

	if len(got) != 1 {
		t.Fatalf("len(emitted) = %d, want 1", len(got))
	}
	if got[0].SegmentID != "source-1" {
		t.Fatalf("emitted segment id = %q, want source-1", got[0].SegmentID)
	}
	current := stabilizer.Current()
	if len(current) != 1 {
		t.Fatalf("len(current) = %d, want 1", len(current))
	}
	if current[0].SourceText != "hello" || current[0].Text != "你好" {
		t.Fatalf("current segment = %+v", current[0])
	}
}

func TestStabilizerPersistsFinalSegmentOnce(t *testing.T) {
	stabilizer := NewStabilizer(StabilizerOptions{})

	first := stabilizer.Apply(InterpretationEvent{
		Type:      EventSegmentFinal,
		SegmentID: "seg-2",
		Text:      "你好",
		StartMS:   500,
		EndMS:     900,
	})
	if len(first) != 1 {
		t.Fatalf("len(first emitted) = %d, want 1", len(first))
	}

	duplicate := stabilizer.Apply(InterpretationEvent{
		Type:      EventSegmentFinal,
		SegmentID: "seg-2",
		Text:      "你好",
		StartMS:   500,
		EndMS:     900,
	})
	if len(duplicate) != 0 {
		t.Fatalf("len(duplicate emitted) = %d, want 0", len(duplicate))
	}

	lateSource := stabilizer.Apply(InterpretationEvent{
		Type:       EventSegmentFinal,
		SegmentID:  "seg-2",
		SourceText: "hello",
		StartMS:    500,
		EndMS:      900,
	})
	if len(lateSource) != 0 {
		t.Fatalf("len(late source emitted) = %d, want 0", len(lateSource))
	}

	segments := stabilizer.Segments()
	if len(segments) != 1 {
		t.Fatalf("len(segments) = %d, want 1", len(segments))
	}
	if segments[0].Text != "你好" || segments[0].SourceText != "hello" {
		t.Fatalf("persisted segment = %+v", segments[0])
	}
}

func TestStabilizerIgnoresTrivialPartialChurn(t *testing.T) {
	stabilizer := NewStabilizer(StabilizerOptions{})

	stabilizer.Apply(InterpretationEvent{
		Type:      EventSegmentPartial,
		SegmentID: "seg-3",
		Text:      "same",
		StartMS:   100,
		EndMS:     200,
	})
	got := stabilizer.Apply(InterpretationEvent{
		Type:      EventSegmentPartial,
		SegmentID: "seg-3",
		Text:      "same",
		StartMS:   100,
		EndMS:     200,
	})

	if len(got) != 0 {
		t.Fatalf("len(emitted) = %d, want 0 for unchanged partial", len(got))
	}
}

func TestStabilizerAppliesLineLengthAndDurationGuards(t *testing.T) {
	stabilizer := NewStabilizer(StabilizerOptions{
		MaxLineRunes:         4,
		MaxSegmentDurationMS: 300,
	})

	got := stabilizer.Apply(InterpretationEvent{
		Type:      EventSegmentFinal,
		SegmentID: "seg-4",
		Text:      "abcdef",
		StartMS:   1000,
		EndMS:     2000,
	})
	if len(got) != 1 {
		t.Fatalf("len(emitted) = %d, want 1", len(got))
	}
	if got[0].Text != "abcd" {
		t.Fatalf("text = %q, want abcd", got[0].Text)
	}
	if got[0].EndMS != 1300 {
		t.Fatalf("end_ms = %d, want 1300", got[0].EndMS)
	}
}

func TestStabilizerPassesThroughNonSegmentEvents(t *testing.T) {
	stabilizer := NewStabilizer(StabilizerOptions{})

	got := stabilizer.Apply(InterpretationEvent{
		Type:    EventSessionError,
		Code:    "provider_error",
		Message: "failed",
	})
	if len(got) != 1 {
		t.Fatalf("len(emitted) = %d, want 1", len(got))
	}
	if got[0].Type != EventSessionError {
		t.Fatalf("type = %q, want %q", got[0].Type, EventSessionError)
	}
}
