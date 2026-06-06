package subtitle

type EventType string

const (
	EventSegmentPartial  EventType = "segment.partial"
	EventSegmentFinal    EventType = "segment.final"
	EventSegmentRevision EventType = "segment.revision"
	EventAudioDelta      EventType = "audio.delta"
	EventSessionState    EventType = "session.state"
	EventSessionError    EventType = "session.error"
)

type AudioCodec string

const (
	CodecPCM     AudioCodec = "pcm"
	CodecOggOpus AudioCodec = "ogg_opus"
)

type InterpretationEvent struct {
	Type          EventType  `json:"type"`
	SegmentID     string     `json:"segmentId,omitempty"`
	Text          string     `json:"text,omitempty"`
	SourceText    string     `json:"sourceText,omitempty"`
	StartMS       int64      `json:"startMs,omitempty"`
	EndMS         int64      `json:"endMs,omitempty"`
	Before        string     `json:"before,omitempty"`
	After         string     `json:"after,omitempty"`
	Reason        string     `json:"reason,omitempty"`
	Audio         []byte     `json:"audio,omitempty"`
	Codec         AudioCodec `json:"codec,omitempty"`
	State         string     `json:"state,omitempty"`
	Code          string     `json:"code,omitempty"`
	Message       string     `json:"message,omitempty"`
	ProviderLogID string     `json:"providerLogId,omitempty"`
}

func NewAudioDelta(segmentID string, audio []byte, codec AudioCodec) InterpretationEvent {
	return InterpretationEvent{
		Type:      EventAudioDelta,
		SegmentID: segmentID,
		Audio:     cloneBytes(audio),
		Codec:     codec,
	}
}

func cloneBytes(values []byte) []byte {
	if len(values) == 0 {
		return nil
	}
	copyValues := make([]byte, len(values))
	copy(copyValues, values)
	return copyValues
}
