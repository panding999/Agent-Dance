package audio

import (
	"encoding/binary"
	"errors"
	"sync"
)

const BrowserFrameHeaderSize = 12

var (
	ErrFrameTooShort = errors.New("browser audio frame is shorter than header")
	ErrEmptyPCM      = errors.New("browser audio frame has no pcm payload")
	ErrMisalignedPCM = errors.New("browser audio frame pcm payload is not int16 aligned")
)

type PCMFrame struct {
	Sequence    uint32
	TimestampMS uint64
	PCM         []byte
}

func ParseBrowserFrame(raw []byte) (PCMFrame, error) {
	if len(raw) < BrowserFrameHeaderSize {
		return PCMFrame{}, ErrFrameTooShort
	}

	pcm := raw[BrowserFrameHeaderSize:]
	if len(pcm) == 0 {
		return PCMFrame{}, ErrEmptyPCM
	}
	if len(pcm)%2 != 0 {
		return PCMFrame{}, ErrMisalignedPCM
	}

	return PCMFrame{
		Sequence:    binary.LittleEndian.Uint32(raw[0:4]),
		TimestampMS: binary.LittleEndian.Uint64(raw[4:12]),
		PCM:         cloneBytes(pcm),
	}, nil
}

type ChunkCache struct {
	mu       sync.Mutex
	capacity int
	frames   []PCMFrame
}

func NewChunkCache(capacity int) *ChunkCache {
	if capacity < 1 {
		capacity = 1
	}
	return &ChunkCache{capacity: capacity}
}

func (c *ChunkCache) Add(frame PCMFrame) {
	c.mu.Lock()
	defer c.mu.Unlock()

	frame.PCM = cloneBytes(frame.PCM)
	c.frames = append(c.frames, frame)
	if len(c.frames) > c.capacity {
		c.frames = c.frames[len(c.frames)-c.capacity:]
	}
}

func (c *ChunkCache) Recent() []PCMFrame {
	c.mu.Lock()
	defer c.mu.Unlock()

	frames := make([]PCMFrame, len(c.frames))
	for i, frame := range c.frames {
		frame.PCM = cloneBytes(frame.PCM)
		frames[i] = frame
	}
	return frames
}

func cloneBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	cloned := make([]byte, len(value))
	copy(cloned, value)
	return cloned
}
