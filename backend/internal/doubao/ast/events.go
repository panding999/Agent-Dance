package ast

type ProviderEvent struct {
	Event          EventType
	SegmentID      string
	Text           string
	Data           []byte
	AudioCodec     string
	StartTimeMS    int64
	EndTimeMS      int64
	SpeakerChanged bool
	Usage          *Usage
	Error          *ProviderError
}

type Usage struct {
	InputAudioSeconds  float64
	OutputAudioSeconds float64
}

type ProviderError struct {
	Code    string
	Message string
	LogID   string
}
