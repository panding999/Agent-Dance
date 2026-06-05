package audio

import (
	"errors"
	"testing"
)

func TestPacketizerSplitsExactly80msPackets(t *testing.T) {
	packetizer, err := NewPacketizer(DefaultPCMFormat)
	if err != nil {
		t.Fatalf("NewPacketizer() error = %v", err)
	}

	packets, err := packetizer.Push(PCMFrame{
		Sequence:    1,
		TimestampMS: 2000,
		PCM:         makeSequentialPCM(DoubaoPacketBytes * 2),
	})
	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	if len(packets) != 2 {
		t.Fatalf("len(packets) = %d, want 2", len(packets))
	}
	if packets[0].Sequence != 0 {
		t.Fatalf("packet 0 sequence = %d, want 0", packets[0].Sequence)
	}
	if packets[0].TimestampMS != 2000 {
		t.Fatalf("packet 0 timestamp = %d, want 2000", packets[0].TimestampMS)
	}
	if len(packets[0].PCM) != DoubaoPacketBytes {
		t.Fatalf("packet 0 bytes = %d, want %d", len(packets[0].PCM), DoubaoPacketBytes)
	}
	if packets[1].Sequence != 1 {
		t.Fatalf("packet 1 sequence = %d, want 1", packets[1].Sequence)
	}
	if packets[1].TimestampMS != 2080 {
		t.Fatalf("packet 1 timestamp = %d, want 2080", packets[1].TimestampMS)
	}

	packets[0].PCM[0] = 99
	next, err := packetizer.Push(PCMFrame{
		Sequence:    2,
		TimestampMS: 2160,
		PCM:         makeSequentialPCM(DoubaoPacketBytes),
	})
	if err != nil {
		t.Fatalf("Push() second frame error = %v", err)
	}
	if next[0].PCM[0] == 99 {
		t.Fatal("packet PCM shares mutable memory with previous caller")
	}
}

func TestDoubaoPacketBytesMatches80msInt16Mono(t *testing.T) {
	if DoubaoPacketBytes != 2560 {
		t.Fatalf("DoubaoPacketBytes = %d, want 2560", DoubaoPacketBytes)
	}
}

func TestPacketizerCarriesRemainderAcrossFrames(t *testing.T) {
	packetizer, err := NewPacketizer(DefaultPCMFormat)
	if err != nil {
		t.Fatalf("NewPacketizer() error = %v", err)
	}

	first, err := packetizer.Push(PCMFrame{
		Sequence:    1,
		TimestampMS: 3000,
		PCM:         makeSequentialPCM(DoubaoPacketBytes / 2),
	})
	if err != nil {
		t.Fatalf("Push() first frame error = %v", err)
	}
	if len(first) != 0 {
		t.Fatalf("len(first packets) = %d, want 0", len(first))
	}
	if packetizer.BufferedBytes() != DoubaoPacketBytes/2 {
		t.Fatalf("BufferedBytes() = %d, want %d", packetizer.BufferedBytes(), DoubaoPacketBytes/2)
	}

	second, err := packetizer.Push(PCMFrame{
		Sequence:    2,
		TimestampMS: 3040,
		PCM:         makeSequentialPCM(DoubaoPacketBytes / 2),
	})
	if err != nil {
		t.Fatalf("Push() second frame error = %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("len(second packets) = %d, want 1", len(second))
	}
	if second[0].TimestampMS != 3000 {
		t.Fatalf("packet timestamp = %d, want 3000", second[0].TimestampMS)
	}
	if packetizer.BufferedBytes() != 0 {
		t.Fatalf("BufferedBytes() = %d, want 0", packetizer.BufferedBytes())
	}
}

func TestPacketizerRejectsUnsupportedFormat(t *testing.T) {
	_, err := NewPacketizer(PCMFormat{
		SampleRateHz:  48000,
		BitsPerSample: 16,
		Channels:      1,
	})
	if !errors.Is(err, ErrUnsupportedPCMFormat) {
		t.Fatalf("NewPacketizer() error = %v, want ErrUnsupportedPCMFormat", err)
	}
}

func TestPacketizerRejectsMisalignedPCM(t *testing.T) {
	packetizer, err := NewPacketizer(DefaultPCMFormat)
	if err != nil {
		t.Fatalf("NewPacketizer() error = %v", err)
	}

	_, err = packetizer.Push(PCMFrame{
		Sequence:    1,
		TimestampMS: 100,
		PCM:         []byte{1, 2, 3},
	})
	if !errors.Is(err, ErrMisalignedPCM) {
		t.Fatalf("Push() error = %v, want ErrMisalignedPCM", err)
	}
}

func makeSequentialPCM(size int) []byte {
	pcm := make([]byte, size)
	for i := range pcm {
		pcm[i] = byte(i % 251)
	}
	return pcm
}
