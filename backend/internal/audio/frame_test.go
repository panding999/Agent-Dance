package audio

import (
	"encoding/binary"
	"errors"
	"testing"
)

func TestParseBrowserFrameReadsHeaderAndPCM(t *testing.T) {
	raw := make([]byte, BrowserFrameHeaderSize+4)
	binary.LittleEndian.PutUint32(raw[0:4], 7)
	binary.LittleEndian.PutUint64(raw[4:12], 1234)
	copy(raw[12:], []byte{0x01, 0x00, 0x02, 0x00})

	frame, err := ParseBrowserFrame(raw)
	if err != nil {
		t.Fatalf("ParseBrowserFrame() error = %v", err)
	}

	if frame.Sequence != 7 {
		t.Fatalf("Sequence = %d", frame.Sequence)
	}
	if frame.TimestampMS != 1234 {
		t.Fatalf("TimestampMS = %d", frame.TimestampMS)
	}
	if len(frame.PCM) != 4 {
		t.Fatalf("len(PCM) = %d", len(frame.PCM))
	}
}

func TestParseBrowserFrameRejectsShortFrames(t *testing.T) {
	_, err := ParseBrowserFrame(make([]byte, BrowserFrameHeaderSize-1))
	if !errors.Is(err, ErrFrameTooShort) {
		t.Fatalf("ParseBrowserFrame() error = %v, want ErrFrameTooShort", err)
	}
}

func TestParseBrowserFrameRejectsOddPCMBytes(t *testing.T) {
	raw := make([]byte, BrowserFrameHeaderSize+3)
	binary.LittleEndian.PutUint32(raw[0:4], 1)
	binary.LittleEndian.PutUint64(raw[4:12], 100)

	_, err := ParseBrowserFrame(raw)
	if !errors.Is(err, ErrMisalignedPCM) {
		t.Fatalf("ParseBrowserFrame() error = %v, want ErrMisalignedPCM", err)
	}
}

func TestChunkCacheKeepsMostRecentFrames(t *testing.T) {
	cache := NewChunkCache(2)
	cache.Add(PCMFrame{Sequence: 1, TimestampMS: 10, PCM: []byte{1, 0}})
	cache.Add(PCMFrame{Sequence: 2, TimestampMS: 20, PCM: []byte{2, 0}})
	cache.Add(PCMFrame{Sequence: 3, TimestampMS: 30, PCM: []byte{3, 0}})

	got := cache.Recent()
	if len(got) != 2 {
		t.Fatalf("len(Recent()) = %d, want 2", len(got))
	}
	if got[0].Sequence != 2 || got[1].Sequence != 3 {
		t.Fatalf("Recent sequences = [%d %d], want [2 3]", got[0].Sequence, got[1].Sequence)
	}

	got[0].PCM[0] = 99
	again := cache.Recent()
	if again[0].PCM[0] == 99 {
		t.Fatal("Recent() returned mutable cached PCM slice")
	}
}
