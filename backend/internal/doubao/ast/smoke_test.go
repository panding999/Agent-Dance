package ast

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/panding999/agent-dance/backend/internal/audio"
	"nhooyr.io/websocket"
)

func TestSmokeStartSessionAndShortAudio(t *testing.T) {
	if os.Getenv("RUN_DOUBAO_SMOKE") != "1" {
		t.Skip("set RUN_DOUBAO_SMOKE=1 to run the real Doubao AST smoke test")
	}

	apiKey := strings.TrimSpace(os.Getenv("DOUBAO_API_KEY"))
	appID := strings.TrimSpace(os.Getenv("DOUBAO_APP_ID"))
	appKey := strings.TrimSpace(os.Getenv("DOUBAO_APP_KEY"))
	accessKey := strings.TrimSpace(os.Getenv("DOUBAO_ACCESS_KEY"))
	if !hasCredentials(apiKey, appID, appKey, accessKey) {
		t.Skip("missing Doubao credentials for smoke test")
	}

	client, err := NewClient(ClientOptions{
		Endpoint:   strings.TrimSpace(os.Getenv("DOUBAO_AST_ENDPOINT")),
		APIKey:     apiKey,
		AppID:      appID,
		AppKey:     appKey,
		AccessKey:  accessKey,
		ResourceID: strings.TrimSpace(os.Getenv("DOUBAO_AST_RESOURCE_ID")),
		ModelID:    strings.TrimSpace(os.Getenv("DOUBAO_AST_MODEL_ID")),
		Codec:      ProtobufCodec{},
	})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close(websocket.StatusNormalClosure, "smoke done")
	})

	sessionID := "smoke-" + randomHex(t, 8)
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	if err := client.StartSession(ctx, StartSessionParams{
		SessionID:      sessionID,
		Mode:           ModeS2T,
		SourceLanguage: smokeEnvDefault("DOUBAO_AST_SMOKE_SOURCE_LANGUAGE", "zh"),
		TargetLanguage: smokeEnvDefault("DOUBAO_AST_SMOKE_TARGET_LANGUAGE", "en"),
	}); err != nil {
		t.Fatalf("StartSession() error = %v", err)
	}
	t.Logf("Doubao AST session started request sent, log_id=%s", client.LogID())

	if err := waitForSmokeEvent(ctx, client, EventSessionStarted); err != nil {
		t.Fatalf("wait for SessionStarted: %v", err)
	}

	chunks := smokeAudioChunks(t, 5)
	for i, chunk := range chunks {
		if err := client.SendAudio(ctx, audio.Packet{
			Sequence:    uint64(i),
			TimestampMS: uint64(i * audio.DoubaoPacketDurationMS),
			PCM:         chunk,
		}); err != nil {
			t.Fatalf("SendAudio(%d) error = %v", i, err)
		}
		time.Sleep(time.Duration(audio.DoubaoPacketDurationMS) * time.Millisecond)
	}

	if err := client.FinishSession(ctx); err != nil {
		t.Fatalf("FinishSession() error = %v", err)
	}
}

func waitForSmokeEvent(ctx context.Context, client *Client, want EventType) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-client.Errors():
			if err != nil {
				return err
			}
		case event := <-client.Events():
			if event.Event == want {
				return nil
			}
			if event.Event == EventSessionFailed {
				if event.Error != nil {
					return errors.New(event.Error.Message)
				}
				return errors.New("doubao ast session failed")
			}
		}
	}
}

func smokeAudioChunks(t *testing.T, maxChunks int) [][]byte {
	t.Helper()

	if path := strings.TrimSpace(os.Getenv("DOUBAO_AST_SMOKE_AUDIO")); path != "" {
		if chunks := chunksFromAudioFile(t, path, maxChunks); len(chunks) > 0 {
			return chunks
		}
	}
	return generatedToneChunks(maxChunks)
}

func chunksFromAudioFile(t *testing.T, path string, maxChunks int) [][]byte {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read DOUBAO_AST_SMOKE_AUDIO: %v", err)
	}
	pcm := content
	if strings.HasPrefix(string(content), "RIFF") {
		pcm = wavPCMData(t, content)
	}
	if len(pcm) == 0 {
		t.Fatal("smoke audio is empty")
	}
	return splitSmokeChunks(pcm, maxChunks)
}

func wavPCMData(t *testing.T, content []byte) []byte {
	t.Helper()

	offset := 12
	for offset+8 <= len(content) {
		chunkID := string(content[offset : offset+4])
		chunkSize := int(binary.LittleEndian.Uint32(content[offset+4 : offset+8]))
		dataStart := offset + 8
		dataEnd := dataStart + chunkSize
		if dataEnd > len(content) {
			t.Fatal("invalid wav data chunk")
		}
		if chunkID == "data" {
			return content[dataStart:dataEnd]
		}
		offset = dataEnd
		if offset%2 == 1 {
			offset++
		}
	}
	t.Fatal("wav file does not contain a data chunk")
	return nil
}

func generatedToneChunks(maxChunks int) [][]byte {
	totalSamples := audio.RequiredSampleRateHz * audio.DoubaoPacketDurationMS / 1000 * maxChunks
	pcm := make([]byte, totalSamples*2)
	for i := 0; i < totalSamples; i++ {
		sample := int16(math.Sin(2*math.Pi*440*float64(i)/float64(audio.RequiredSampleRateHz)) * 12000)
		binary.LittleEndian.PutUint16(pcm[i*2:i*2+2], uint16(sample))
	}
	return splitSmokeChunks(pcm, maxChunks)
}

func splitSmokeChunks(pcm []byte, maxChunks int) [][]byte {
	limit := audio.DoubaoPacketBytes * maxChunks
	if len(pcm) > limit {
		pcm = pcm[:limit]
	}

	chunks := make([][]byte, 0, (len(pcm)+audio.DoubaoPacketBytes-1)/audio.DoubaoPacketBytes)
	for len(pcm) > 0 {
		size := audio.DoubaoPacketBytes
		if len(pcm) < size {
			size = len(pcm)
		}
		chunk := make([]byte, size)
		copy(chunk, pcm[:size])
		chunks = append(chunks, chunk)
		pcm = pcm[size:]
	}
	return chunks
}

func smokeEnvDefault(key string, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func randomHex(t *testing.T, bytesLen int) string {
	t.Helper()

	buf := make([]byte, bytesLen)
	if _, err := rand.Read(buf); err != nil {
		t.Fatalf("read random bytes: %v", err)
	}
	return hex.EncodeToString(buf)
}
