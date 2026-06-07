package ast

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	eventpb "code.byted.org/data-speech/wsclientsdk/protogen/common/event"
	rpcmetapb "code.byted.org/data-speech/wsclientsdk/protogen/common/rpcmeta"
	astpb "code.byted.org/data-speech/wsclientsdk/protogen/products/understanding/ast"
	basepb "code.byted.org/data-speech/wsclientsdk/protogen/products/understanding/base"
	"google.golang.org/protobuf/proto"
)

type ProtobufCodec struct{}

var _ Codec = ProtobufCodec{}

func (ProtobufCodec) EncodeStartSession(req StartSessionRequest) ([]byte, error) {
	pbReq := &astpb.TranslateRequest{
		RequestMeta: &rpcmetapb.RequestMeta{
			SessionID: req.RequestMeta.SessionID,
		},
		Event:       eventpb.Type(req.Event),
		User:        encodeUser(req.User),
		SourceAudio: encodeAudio(req.SourceAudio, nil),
		Request: &astpb.ReqParams{
			Mode:           string(req.Request.Mode),
			SourceLanguage: req.Request.SourceLanguage,
			TargetLanguage: req.Request.TargetLanguage,
			SpeakerId:      req.Request.SpeakerID,
			SpeechRate:     int32(req.Request.SpeechRate),
			Corpus:         encodeCorpus(req.Request.Corpus),
		},
	}
	if req.TargetAudio != nil {
		pbReq.TargetAudio = encodeAudio(*req.TargetAudio, nil)
	}
	return proto.Marshal(pbReq)
}

func (ProtobufCodec) EncodeTaskRequest(req TaskRequest) ([]byte, error) {
	pbReq := &astpb.TranslateRequest{
		RequestMeta: &rpcmetapb.RequestMeta{
			SessionID: req.SessionID,
			Sequence:  int32(req.Packet.Sequence),
		},
		Event: eventpb.Type(req.Event),
		SourceAudio: &basepb.Audio{
			BinaryData: cloneBytes(req.Packet.PCM),
		},
	}
	return proto.Marshal(pbReq)
}

func (ProtobufCodec) EncodeFinishSession(req FinishSessionRequest) ([]byte, error) {
	pbReq := &astpb.TranslateRequest{
		RequestMeta: &rpcmetapb.RequestMeta{
			SessionID: req.SessionID,
		},
		Event: eventpb.Type(req.Event),
	}
	return proto.Marshal(pbReq)
}

func (ProtobufCodec) DecodeProviderEvent(payload []byte) (ProviderEvent, error) {
	var resp astpb.TranslateResponse
	if err := proto.Unmarshal(payload, &resp); err != nil {
		return ProviderEvent{}, err
	}

	meta := resp.GetResponseMeta()
	event := ProviderEvent{
		Event:          EventType(resp.GetEvent()),
		SegmentID:      responseSegmentID(meta),
		Text:           resp.GetText(),
		Data:           cloneBytes(resp.GetData()),
		StartTimeMS:    int64(resp.GetStartTime()),
		EndTimeMS:      int64(resp.GetEndTime()),
		SpeakerChanged: resp.GetSpkChg(),
	}
	if event.Event == EventUsageResponse {
		event.Usage = usageFromBilling(meta.GetBilling())
	}
	if event.Event == EventSessionFailed || isProviderError(meta) {
		event.Error = providerError(meta)
	}
	return event, nil
}

func encodeUser(user UserConfig) *basepb.User {
	if user == (UserConfig{}) {
		return nil
	}
	return &basepb.User{
		Uid:        user.UID,
		Did:        user.DID,
		Platform:   user.Platform,
		SdkVersion: user.SDKVersion,
	}
}

func encodeAudio(config AudioConfig, binaryData []byte) *basepb.Audio {
	return &basepb.Audio{
		Format:     config.Format,
		Codec:      config.Codec,
		Rate:       int32(config.Rate),
		Bits:       int32(config.Bits),
		Channel:    int32(config.Channel),
		BinaryData: cloneBytes(binaryData),
	}
}

func encodeCorpus(corpus Corpus) *basepb.Corpus {
	if isEmptyCorpus(corpus) {
		return nil
	}
	return &basepb.Corpus{
		HotWordsList:          cloneStrings(corpus.HotWordsList),
		BoostingTableId:       corpus.BoostingTableID,
		BoostingTableName:     corpus.BoostingTableName,
		CorrectWords:          encodeCorrectWords(corpus.CorrectWords),
		RegexCorrectTableId:   corpus.RegexCorrectTableID,
		RegexCorrectTableName: corpus.RegexCorrectTableName,
		GlossaryList:          cloneStringMap(corpus.GlossaryList),
		GlossaryTableId:       corpus.GlossaryTableID,
		GlossaryTableName:     corpus.GlossaryTableName,
	}
}

func encodeCorrectWords(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	payload, err := json.Marshal(values)
	if err != nil {
		return ""
	}
	return string(payload)
}

func isEmptyCorpus(corpus Corpus) bool {
	return len(corpus.HotWordsList) == 0 &&
		strings.TrimSpace(corpus.BoostingTableID) == "" &&
		strings.TrimSpace(corpus.BoostingTableName) == "" &&
		len(corpus.CorrectWords) == 0 &&
		strings.TrimSpace(corpus.RegexCorrectTableID) == "" &&
		strings.TrimSpace(corpus.RegexCorrectTableName) == "" &&
		len(corpus.GlossaryList) == 0 &&
		strings.TrimSpace(corpus.GlossaryTableID) == "" &&
		strings.TrimSpace(corpus.GlossaryTableName) == ""
}

func responseSegmentID(meta *rpcmetapb.ResponseMeta) string {
	if meta == nil {
		return ""
	}
	sessionID := strings.TrimSpace(meta.GetSessionID())
	if meta.GetSequence() == 0 {
		return sessionID
	}
	if sessionID == "" {
		return strconv.FormatInt(int64(meta.GetSequence()), 10)
	}
	return fmt.Sprintf("%s:%d", sessionID, meta.GetSequence())
}

func isProviderError(meta *rpcmetapb.ResponseMeta) bool {
	if meta == nil {
		return false
	}
	status := meta.GetStatusCode()
	return status != 0 && status != 20000000 && status != 21000
}

func providerError(meta *rpcmetapb.ResponseMeta) *ProviderError {
	if meta == nil {
		return &ProviderError{}
	}
	code := ""
	if meta.GetStatusCode() != 0 {
		code = strconv.FormatInt(int64(meta.GetStatusCode()), 10)
	}
	return &ProviderError{
		Code:    code,
		Message: strings.TrimSpace(meta.GetMessage()),
	}
}

func usageFromBilling(billing *rpcmetapb.Billing) *Usage {
	if billing == nil {
		return nil
	}
	return &Usage{
		InputAudioSeconds: float64(billing.GetDurationMsec()) / 1000,
	}
}
