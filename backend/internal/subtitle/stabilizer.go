package subtitle

const (
	defaultMergeWindowMS        int64 = 500
	defaultMaxLineRunes               = 160
	defaultMaxSegmentDurationMS int64 = 15000
)

type StabilizerOptions struct {
	MergeWindowMS        int64
	MaxLineRunes         int
	MaxSegmentDurationMS int64
}

type Segment struct {
	ID         string
	Text       string
	SourceText string
	StartMS    int64
	EndMS      int64
	Final      bool
}

type Stabilizer struct {
	options         StabilizerOptions
	current         map[string]*Segment
	aliases         map[string]string
	lastPartial     map[string]InterpretationEvent
	history         []Segment
	historyIndex    map[string]int
	finalized       map[string]bool
	segmentSequence int
}

func NewStabilizer(options StabilizerOptions) *Stabilizer {
	if options.MergeWindowMS <= 0 {
		options.MergeWindowMS = defaultMergeWindowMS
	}
	if options.MaxLineRunes <= 0 {
		options.MaxLineRunes = defaultMaxLineRunes
	}
	if options.MaxSegmentDurationMS <= 0 {
		options.MaxSegmentDurationMS = defaultMaxSegmentDurationMS
	}

	return &Stabilizer{
		options:      options,
		current:      make(map[string]*Segment),
		aliases:      make(map[string]string),
		lastPartial:  make(map[string]InterpretationEvent),
		historyIndex: make(map[string]int),
		finalized:    make(map[string]bool),
	}
}

func (s *Stabilizer) Apply(event InterpretationEvent) []InterpretationEvent {
	switch event.Type {
	case EventSegmentPartial:
		return s.applyPartial(event)
	case EventSegmentFinal:
		return s.applyFinal(event)
	default:
		return []InterpretationEvent{cloneEvent(event)}
	}
}

func (s *Stabilizer) Current() []Segment {
	segments := make([]Segment, 0, len(s.current))
	for _, segment := range s.current {
		segments = append(segments, cloneSegment(*segment))
	}
	return segments
}

func (s *Stabilizer) Segments() []Segment {
	segments := make([]Segment, len(s.history))
	copy(segments, s.history)
	return segments
}

func (s *Stabilizer) applyPartial(event InterpretationEvent) []InterpretationEvent {
	segmentID := s.resolveSegmentID(event)
	segment := s.upsertSegment(segmentID)
	s.mergeSegmentEvent(segment, event)

	stabilized := s.eventFromSegment(EventSegmentPartial, segment)
	if last, ok := s.lastPartial[segmentID]; ok && samePartial(last, stabilized) {
		return nil
	}
	s.lastPartial[segmentID] = stabilized
	return []InterpretationEvent{stabilized}
}

func (s *Stabilizer) applyFinal(event InterpretationEvent) []InterpretationEvent {
	segmentID := s.resolveSegmentID(event)
	segment := s.upsertSegment(segmentID)
	s.mergeSegmentEvent(segment, event)
	segment.Final = true

	if index, ok := s.historyIndex[segmentID]; ok {
		s.history[index] = cloneSegment(*segment)
	} else {
		s.historyIndex[segmentID] = len(s.history)
		s.history = append(s.history, cloneSegment(*segment))
	}

	if s.finalized[segmentID] {
		return nil
	}
	s.finalized[segmentID] = true
	return []InterpretationEvent{s.eventFromSegment(EventSegmentFinal, segment)}
}

func (s *Stabilizer) resolveSegmentID(event InterpretationEvent) string {
	incomingID := event.SegmentID
	if incomingID == "" {
		s.segmentSequence++
		incomingID = "segment-" + intString(s.segmentSequence)
	}
	if canonicalID, ok := s.aliases[incomingID]; ok {
		return canonicalID
	}
	if _, ok := s.current[incomingID]; ok {
		s.aliases[incomingID] = incomingID
		return incomingID
	}

	if matchID := s.findTimeWindowMatch(event); matchID != "" {
		s.aliases[incomingID] = matchID
		return matchID
	}

	s.aliases[incomingID] = incomingID
	return incomingID
}

func (s *Stabilizer) findTimeWindowMatch(event InterpretationEvent) string {
	for segmentID, segment := range s.current {
		if segment.Final {
			continue
		}
		if withinMergeWindow(segment.StartMS, event.StartMS, s.options.MergeWindowMS) ||
			withinMergeWindow(segment.EndMS, event.EndMS, s.options.MergeWindowMS) ||
			timeRangesOverlap(segment.StartMS, segment.EndMS, event.StartMS, event.EndMS) {
			return segmentID
		}
	}
	return ""
}

func (s *Stabilizer) upsertSegment(segmentID string) *Segment {
	if segment, ok := s.current[segmentID]; ok {
		return segment
	}
	segment := &Segment{ID: segmentID}
	s.current[segmentID] = segment
	return segment
}

func (s *Stabilizer) mergeSegmentEvent(segment *Segment, event InterpretationEvent) {
	if event.Text != "" {
		segment.Text = truncateRunes(event.Text, s.options.MaxLineRunes)
	}
	if event.SourceText != "" {
		segment.SourceText = truncateRunes(event.SourceText, s.options.MaxLineRunes)
	}
	if segment.StartMS == 0 || (event.StartMS != 0 && event.StartMS < segment.StartMS) {
		segment.StartMS = event.StartMS
	}
	if event.EndMS > segment.EndMS {
		segment.EndMS = event.EndMS
	}
	if segment.StartMS != 0 && segment.EndMS-segment.StartMS > s.options.MaxSegmentDurationMS {
		segment.EndMS = segment.StartMS + s.options.MaxSegmentDurationMS
	}
}

func (s *Stabilizer) eventFromSegment(eventType EventType, segment *Segment) InterpretationEvent {
	return InterpretationEvent{
		Type:       eventType,
		SegmentID:  segment.ID,
		Text:       segment.Text,
		SourceText: segment.SourceText,
		StartMS:    segment.StartMS,
		EndMS:      segment.EndMS,
	}
}

func samePartial(left InterpretationEvent, right InterpretationEvent) bool {
	return left.Text == right.Text &&
		left.SourceText == right.SourceText &&
		left.StartMS == right.StartMS &&
		left.EndMS == right.EndMS
}

func withinMergeWindow(left int64, right int64, window int64) bool {
	if left == 0 || right == 0 {
		return false
	}
	diff := left - right
	if diff < 0 {
		diff = -diff
	}
	return diff <= window
}

func timeRangesOverlap(leftStart int64, leftEnd int64, rightStart int64, rightEnd int64) bool {
	if leftStart == 0 || leftEnd == 0 || rightStart == 0 || rightEnd == 0 {
		return false
	}
	return leftStart <= rightEnd && rightStart <= leftEnd
}

func truncateRunes(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

func cloneSegment(segment Segment) Segment {
	return Segment{
		ID:         segment.ID,
		Text:       segment.Text,
		SourceText: segment.SourceText,
		StartMS:    segment.StartMS,
		EndMS:      segment.EndMS,
		Final:      segment.Final,
	}
}

func cloneEvent(event InterpretationEvent) InterpretationEvent {
	event.Audio = cloneBytes(event.Audio)
	return event
}

func intString(value int) string {
	if value == 0 {
		return "0"
	}
	var digits [20]byte
	index := len(digits)
	for value > 0 {
		index--
		digits[index] = byte('0' + value%10)
		value /= 10
	}
	return string(digits[index:])
}
