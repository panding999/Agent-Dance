package audio

import (
	"encoding/binary"
	"errors"
	"sync"
)

const (
	BrowserFrameHeaderSize  = 12
	MaxBrowserFramePCMBytes = RequiredSampleRateHz * RequiredBitsPerSample / 8 * RequiredChannels
)

var (
	ErrFrameTooShort = errors.New("browser audio frame is shorter than header")
	ErrFrameTooLarge = errors.New("browser audio frame pcm payload is too large")
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
	if len(pcm) > MaxBrowserFramePCMBytes {
		return PCMFrame{}, ErrFrameTooLarge
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

type SessionChunkCache struct {
	mu       sync.Mutex
	capacity int
	caches   map[string]*ChunkCache
}

func NewSessionChunkCache(capacity int) *SessionChunkCache {
	if capacity < 1 {
		capacity = 1
	}
	return &SessionChunkCache{
		capacity: capacity,
		caches:   map[string]*ChunkCache{},
	}
}

func (c *SessionChunkCache) Add(sessionID string, frame PCMFrame) {
	cache := c.cacheFor(sessionID)
	cache.Add(frame)
}

func (c *SessionChunkCache) Recent(sessionID string) []PCMFrame {
	c.mu.Lock()
	cache := c.caches[sessionID]
	c.mu.Unlock()
	if cache == nil {
		return nil
	}
	return cache.Recent()
}

func (c *SessionChunkCache) Delete(sessionID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.caches, sessionID)
}

func (c *SessionChunkCache) cacheFor(sessionID string) *ChunkCache {
	c.mu.Lock()
	defer c.mu.Unlock()

	cache := c.caches[sessionID]
	if cache == nil {
		cache = NewChunkCache(c.capacity)
		c.caches[sessionID] = cache
	}
	return cache
}

func cloneBytes(value []byte) []byte {
	if len(value) == 0 {
		return nil
	}
	cloned := make([]byte, len(value))
	copy(cloned, value)
	return cloned
}
