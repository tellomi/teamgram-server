package core

import (
	"bytes"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/jsonx"
)

func voiceNote() *mtproto.Message {
	return mtproto.MakeTLMessage(&mtproto.Message{
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: mtproto.MakeTLDocument(&mtproto.Document{
				MimeType: "audio/ogg",
				Attributes: []*mtproto.DocumentAttribute{
					mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{
						Voice: true, Duration: 1,
					}).To_DocumentAttribute(),
				},
			}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
}

func roundVideo() *mtproto.Message {
	return mtproto.MakeTLMessage(&mtproto.Message{
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: mtproto.MakeTLDocument(&mtproto.Document{
				MimeType: "video/mp4",
				Attributes: []*mtproto.DocumentAttribute{
					mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
						RoundMessage: true, W: 240, H: 240, Duration: 1,
					}).To_DocumentAttribute(),
				},
			}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
}

func plainVideo() *mtproto.Message {
	return mtproto.MakeTLMessage(&mtproto.Message{
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: mtproto.MakeTLDocument(&mtproto.Document{
				MimeType: "video/mp4",
				Attributes: []*mtproto.DocumentAttribute{
					mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
						W: 320, H: 240, Duration: 2,
					}).To_DocumentAttribute(),
				},
			}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
}

func TestVoiceAndRoundStayInMediaAudio(t *testing.T) {
	voice := voiceNote()
	round := roundVideo()
	video := plainVideo()
	if mtproto.GetMediaType(voice) != mtproto.MEDIA_AUDIO || mtproto.GetMediaType(round) != mtproto.MEDIA_AUDIO {
		t.Fatal("voice and round video must stay MEDIA_AUDIO so an older messages.search still lists them")
	}
	if mtproto.GetMediaType(video) != mtproto.MEDIA_VIDEOS_ONLY {
		t.Fatal("a normal video is not a round video")
	}
	voiceJSON := mysqlJSONText(mustJSON(t, voice))
	roundJSON := mysqlJSONText(mustJSON(t, round))
	videoJSON := mysqlJSONText(mustJSON(t, video))
	if !bytes.Contains(voiceJSON, []byte(`"voice": true`)) || bytes.Contains(voiceJSON, []byte(`"round_message": true`)) {
		t.Fatalf("voice json %s", voiceJSON)
	}
	if !bytes.Contains(roundJSON, []byte(`"round_message": true`)) || bytes.Contains(roundJSON, []byte(`"voice": true`)) {
		t.Fatalf("round json %s", roundJSON)
	}
	if bytes.Contains(videoJSON, []byte(`"voice": true`)) || bytes.Contains(videoJSON, []byte(`"round_message": true`)) {
		t.Fatalf("plain video json %s", videoJSON)
	}
	if !mtproto.IsVoiceMessage(voice) || mtproto.IsVoiceMessage(round) || mtproto.IsRoundVideoMessage(voice) || !mtproto.IsRoundVideoMessage(round) {
		t.Fatal("attribute classification diverged from the stored marks")
	}
}

func TestAudioKindWhereDoesNotDoubleCount(t *testing.T) {
	where := audioKindWhere(7, 7, 9, true)
	if !strings.Contains(where, "message_filter_type = 2") || strings.Contains(where, "message_filter_type = 9") || strings.Contains(where, "message_filter_type = 11") {
		t.Fatalf("voice where counts a non-audio column: %s", where)
	}
	if !strings.Contains(where, voiceDataMark) || strings.Contains(where, roundDataMark) {
		t.Fatalf("voice where %s", where)
	}
	roundWhere := audioKindWhere(7, 7, 9, false)
	if !strings.Contains(roundWhere, roundDataMark) || strings.Contains(roundWhere, voiceDataMark) {
		t.Fatalf("round where %s", roundWhere)
	}
	if !strings.Contains(where, "user_id = 7") || !strings.Contains(where, "dialog_id1 = 7") || !strings.Contains(where, "dialog_id2 = 9") {
		t.Fatalf("dialog bounds %s", where)
	}
}

// mysqlJSONText is the spacing MySQL uses when a JSON column is read as text.
func mysqlJSONText(raw []byte) []byte {
	return bytes.ReplaceAll(raw, []byte(`":`), []byte(`": `))
}

func mustJSON(t *testing.T, msg *mtproto.Message) []byte {
	t.Helper()
	raw, err := jsonx.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
