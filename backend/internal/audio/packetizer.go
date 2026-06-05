package audio

import (
	"errors"
	"fmt"
)

const (
	RequiredSampleRateHz   = 16000
	RequiredBitsPerSample  = 16
	RequiredChannels       = 1
	DoubaoPacketDurationMS = 80
	DoubaoPacketBytes      = RequiredSampleRateHz * DoubaoPacketDurationMS / 1000 * RequiredBitsPerSample / 8 * RequiredChannels
)

var ErrUnsupportedPCMFormat = errors.New("unsupported pcm format")

type PCMFormat struct {
	SampleRateHz  int
	BitsPerSample int
	Channels      int
}

var DefaultPCMFormat = PCMFormat{
	SampleRateHz:  RequiredSampleRateHz,
	BitsPerSample: RequiredBitsPerSample,
	Channels:      RequiredChannels,
}

type Packet struct {
	Sequence    uint64
	TimestampMS uint64
	PCM         []byte
}

type Packetizer struct {
	format                PCMFormat
	buffer                []byte
	nextPacketSequence    uint64
	nextPacketTimestampMS uint64
	hasBufferedTimestamp  bool
}

func NewPacketizer(format PCMFormat) (*Packetizer, error) {
	if err := validatePCMFormat(format); err != nil {
		return nil, err
	}
	return &Packetizer{format: format}, nil
}

func (p *Packetizer) Push(frame PCMFrame) ([]Packet, error) {
	if len(frame.PCM)%2 != 0 {
		return nil, ErrMisalignedPCM
	}
	if len(frame.PCM) == 0 {
		return nil, ErrEmptyPCM
	}

	if len(p.buffer) == 0 && !p.hasBufferedTimestamp {
		p.nextPacketTimestampMS = frame.TimestampMS
		p.hasBufferedTimestamp = true
	}

	p.buffer = append(p.buffer, frame.PCM...)
	packets := make([]Packet, 0, len(p.buffer)/DoubaoPacketBytes)

	for len(p.buffer) >= DoubaoPacketBytes {
		packetPCM := cloneBytes(p.buffer[:DoubaoPacketBytes])
		packets = append(packets, Packet{
			Sequence:    p.nextPacketSequence,
			TimestampMS: p.nextPacketTimestampMS,
			PCM:         packetPCM,
		})

		p.nextPacketSequence++
		p.nextPacketTimestampMS += DoubaoPacketDurationMS
		p.buffer = p.buffer[DoubaoPacketBytes:]
	}

	if len(p.buffer) == 0 {
		p.hasBufferedTimestamp = false
	}

	return packets, nil
}

func (p *Packetizer) BufferedBytes() int {
	return len(p.buffer)
}

func validatePCMFormat(format PCMFormat) error {
	if format.SampleRateHz != RequiredSampleRateHz ||
		format.BitsPerSample != RequiredBitsPerSample ||
		format.Channels != RequiredChannels {
		return fmt.Errorf(
			"%w: got %dHz/%dbit/%dch, want %dHz/%dbit/%dch",
			ErrUnsupportedPCMFormat,
			format.SampleRateHz,
			format.BitsPerSample,
			format.Channels,
			RequiredSampleRateHz,
			RequiredBitsPerSample,
			RequiredChannels,
		)
	}
	return nil
}
