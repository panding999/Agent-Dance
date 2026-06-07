package subtitle

import (
	"encoding/json"
	"testing"
)

func TestInterpretationEventJSONUsesFrontendFieldNames(t *testing.T) {
	startMS := int64(120)
	endMS := int64(880)
	event := InterpretationEvent{
		Type:       EventSegmentFinal,
		SegmentID:  "seg-1",
		Text:       "你好",
		SourceText: "hello",
		StartMS:    startMS,
		EndMS:      endMS,
	}

	payload, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}

	want := `{"type":"segment.final","segmentId":"seg-1","text":"你好","sourceText":"hello","startMs":120,"endMs":880}`
	if string(payload) != want {
		t.Fatalf("payload = %s, want %s", payload, want)
	}
}

func TestAudioDeltaClonesAudioPayload(t *testing.T) {
	raw := []byte{1, 2, 3}
	event := NewAudioDelta("seg-2", raw, CodecPCM)

	raw[0] = 99
	if event.Audio[0] == 99 {
		t.Fatal("audio delta shares mutable audio payload")
	}
}
