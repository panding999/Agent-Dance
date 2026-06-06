package ast

import (
	"encoding/json"
	"testing"

	eventpb "code.byted.org/data-speech/wsclientsdk/protogen/common/event"
	rpcmetapb "code.byted.org/data-speech/wsclientsdk/protogen/common/rpcmeta"
	astpb "code.byted.org/data-speech/wsclientsdk/protogen/products/understanding/ast"
	"github.com/panding999/agent-dance/backend/internal/audio"
	"google.golang.org/protobuf/proto"
)

func TestProtobufCodecEncodesStartSession(t *testing.T) {
	codec := ProtobufCodec{}

	payload, err := codec.EncodeStartSession(StartSessionRequest{
		RequestMeta: RequestMeta{SessionID: "session-1", ModelID: "seed-liveinterpret-2"},
		Event:       EventStartSession,
		User: UserConfig{
			UID:        "user-1",
			DID:        "device-1",
			Platform:   "web",
			SDKVersion: "test",
		},
		SourceAudio: DefaultSourceAudioConfig(),
		TargetAudio: &AudioConfig{Format: "pcm", Rate: 24000},
		Request: SessionRequest{
			Mode:           ModeS2S,
			SpeakerID:      "zh_female_vv_uranus_bigtts",
			SpeechRate:     12,
			SourceLanguage: "zh",
			TargetLanguage: "en",
			Corpus: Corpus{
				HotWordsList:          []string{"Agent Dance"},
				BoostingTableID:       "boost-id",
				BoostingTableName:     "boost-name",
				CorrectWords:          map[string]string{"接受": "接收"},
				RegexCorrectTableID:   "regex-id",
				RegexCorrectTableName: "regex-name",
				GlossaryList:          map[string]string{"同声传译": "simultaneous interpretation"},
				GlossaryTableID:       "glossary-id",
				GlossaryTableName:     "glossary-name",
			},
		},
	})
	if err != nil {
		t.Fatalf("EncodeStartSession() error = %v", err)
	}

	var got astpb.TranslateRequest
	if err := proto.Unmarshal(payload, &got); err != nil {
		t.Fatalf("unmarshal TranslateRequest: %v", err)
	}

	if got.GetEvent() != eventpb.Type_StartSession {
		t.Fatalf("event = %v, want %v", got.GetEvent(), eventpb.Type_StartSession)
	}
	if got.GetRequestMeta().GetSessionID() != "session-1" {
		t.Fatalf("session_id = %q", got.GetRequestMeta().GetSessionID())
	}
	if got.GetUser().GetUid() != "user-1" || got.GetUser().GetDid() != "device-1" {
		t.Fatalf("user = %+v", got.GetUser())
	}
	if got.GetSourceAudio().GetFormat() != "wav" || got.GetSourceAudio().GetCodec() != "raw" {
		t.Fatalf("source_audio = %+v", got.GetSourceAudio())
	}
	if got.GetTargetAudio().GetFormat() != "pcm" || got.GetTargetAudio().GetRate() != 24000 {
		t.Fatalf("target_audio = %+v", got.GetTargetAudio())
	}
	if got.GetRequest().GetMode() != "s2s" || got.GetRequest().GetSourceLanguage() != "zh" || got.GetRequest().GetTargetLanguage() != "en" {
		t.Fatalf("request = %+v", got.GetRequest())
	}
	if got.GetRequest().GetSpeakerId() != "zh_female_vv_uranus_bigtts" || got.GetRequest().GetSpeechRate() != 12 {
		t.Fatalf("speaker/speech_rate = %q/%d", got.GetRequest().GetSpeakerId(), got.GetRequest().GetSpeechRate())
	}
	if got.GetRequest().GetCorpus().GetHotWordsList()[0] != "Agent Dance" {
		t.Fatalf("hot_words_list = %#v", got.GetRequest().GetCorpus().GetHotWordsList())
	}
	if got.GetRequest().GetCorpus().GetGlossaryList()["同声传译"] != "simultaneous interpretation" {
		t.Fatalf("glossary_list = %#v", got.GetRequest().GetCorpus().GetGlossaryList())
	}
	var correctWords map[string]string
	if err := json.Unmarshal([]byte(got.GetRequest().GetCorpus().GetCorrectWords()), &correctWords); err != nil {
		t.Fatalf("correct_words is not JSON: %v", err)
	}
	if correctWords["接受"] != "接收" {
		t.Fatalf("correct_words = %#v", correctWords)
	}
	if got.GetRequest().GetCorpus().GetRegexCorrectTableId() != "regex-id" || got.GetRequest().GetCorpus().GetGlossaryTableId() != "glossary-id" {
		t.Fatalf("corpus tables = %+v", got.GetRequest().GetCorpus())
	}
}

func TestProtobufCodecEncodesTaskAndFinishSession(t *testing.T) {
	codec := ProtobufCodec{}

	taskPayload, err := codec.EncodeTaskRequest(TaskRequest{
		Event:     EventTaskRequest,
		SessionID: "session-2",
		Packet: audio.Packet{
			Sequence:    3,
			TimestampMS: 240,
			PCM:         []byte{1, 0, 2, 0},
		},
	})
	if err != nil {
		t.Fatalf("EncodeTaskRequest() error = %v", err)
	}

	var task astpb.TranslateRequest
	if err := proto.Unmarshal(taskPayload, &task); err != nil {
		t.Fatalf("unmarshal task request: %v", err)
	}
	if task.GetEvent() != eventpb.Type_TaskRequest {
		t.Fatalf("task event = %v", task.GetEvent())
	}
	if task.GetRequestMeta().GetSessionID() != "session-2" {
		t.Fatalf("task session_id = %q", task.GetRequestMeta().GetSessionID())
	}
	if task.GetRequestMeta().GetSequence() != 3 {
		t.Fatalf("task sequence = %d", task.GetRequestMeta().GetSequence())
	}
	if string(task.GetSourceAudio().GetBinaryData()) != string([]byte{1, 0, 2, 0}) {
		t.Fatalf("task audio data = %v", task.GetSourceAudio().GetBinaryData())
	}

	finishPayload, err := codec.EncodeFinishSession(FinishSessionRequest{
		Event:     EventFinishSession,
		SessionID: "session-2",
	})
	if err != nil {
		t.Fatalf("EncodeFinishSession() error = %v", err)
	}

	var finish astpb.TranslateRequest
	if err := proto.Unmarshal(finishPayload, &finish); err != nil {
		t.Fatalf("unmarshal finish request: %v", err)
	}
	if finish.GetEvent() != eventpb.Type_FinishSession || finish.GetRequestMeta().GetSessionID() != "session-2" {
		t.Fatalf("finish request = %+v", &finish)
	}
}

func TestProtobufCodecDecodesProviderEvents(t *testing.T) {
	codec := ProtobufCodec{}

	payload, err := proto.Marshal(&astpb.TranslateResponse{
		ResponseMeta: &rpcmetapb.ResponseMeta{
			SessionID:  "session-3",
			Sequence:   9,
			StatusCode: 20000000,
			Message:    "OK",
		},
		Event:     eventpb.Type_TranslationSubtitleEnd,
		Text:      "hello",
		Data:      []byte{3, 2, 1},
		StartTime: 120,
		EndTime:   360,
		SpkChg:    true,
	})
	if err != nil {
		t.Fatalf("marshal provider response: %v", err)
	}

	got, err := codec.DecodeProviderEvent(payload)
	if err != nil {
		t.Fatalf("DecodeProviderEvent() error = %v", err)
	}
	if got.Event != EventTranslationSubtitleEnd || got.Text != "hello" || got.SegmentID != "session-3:9" {
		t.Fatalf("provider event = %+v", got)
	}
	if got.StartTimeMS != 120 || got.EndTimeMS != 360 || !got.SpeakerChanged {
		t.Fatalf("provider timing/speaker = %+v", got)
	}
	if string(got.Data) != string([]byte{3, 2, 1}) {
		t.Fatalf("provider data = %v", got.Data)
	}
}

func TestProtobufCodecDecodesSessionFailed(t *testing.T) {
	codec := ProtobufCodec{}

	payload, err := proto.Marshal(&astpb.TranslateResponse{
		ResponseMeta: &rpcmetapb.ResponseMeta{
			SessionID:  "session-4",
			StatusCode: 45000001,
			Message:    "invalid params",
		},
		Event: eventpb.Type_SessionFailed,
		Text:  "failed text",
	})
	if err != nil {
		t.Fatalf("marshal failed response: %v", err)
	}

	got, err := codec.DecodeProviderEvent(payload)
	if err != nil {
		t.Fatalf("DecodeProviderEvent() error = %v", err)
	}
	if got.Event != EventSessionFailed || got.Error == nil {
		t.Fatalf("provider event = %+v", got)
	}
	if got.Error.Code != "45000001" || got.Error.Message != "invalid params" {
		t.Fatalf("provider error = %+v", got.Error)
	}
}
