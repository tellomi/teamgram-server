package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
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

func TestVoiceCounterVisibleOnTheFilterColumn(t *testing.T) {
	voice := voiceNote()
	round := roundVideo()
	if got := mtproto.GetMediaType(voice); got != mtproto.MEDIA_VOICE_FILE {
		t.Fatalf("voice filter type %d", got)
	}
	if got := mtproto.GetMediaType(round); got != mtproto.MEDIA_ROUND_FILE {
		t.Fatalf("round filter type %d", got)
	}
	plain := mtproto.MakeTLMessage(&mtproto.Message{
		Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
			Document: mtproto.MakeTLDocument(&mtproto.Document{
				MimeType: "video/mp4",
				Attributes: []*mtproto.DocumentAttribute{
					mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{W: 320, H: 240, Duration: 2}).To_DocumentAttribute(),
				},
			}).To_Document(),
		}).To_MessageMedia(),
	}).To_Message()
	if got := mtproto.GetMediaType(plain); got != mtproto.MEDIA_VIDEOS_ONLY {
		t.Fatalf("plain video filter type %d", got)
	}
	// The row the sender just wrote. Counting uses that column, not a
	// later parse of message_data.
	fresh := []visibleMedia{
		{FilterType: mtproto.GetMediaType(voice), Message: voice},
		{FilterType: mtproto.GetMediaType(round), Message: round},
	}
	if got := countVisibleMedia(fresh, mtproto.MEDIA_VOICE_FILE); got != 1 {
		t.Fatalf("voice %d", got)
	}
	if got := countVisibleMedia(fresh, mtproto.MEDIA_ROUND_FILE); got != 1 {
		t.Fatalf("round %d", got)
	}
	if got := countVisibleMedia(fresh, mtproto.MEDIA_AUDIO); got != 2 {
		t.Fatalf("round voice %d", got)
	}
	if countVisibleMedia(fresh[:1], mtproto.MEDIA_ROUND_FILE) != 0 ||
		countVisibleMedia(fresh[1:], mtproto.MEDIA_VOICE_FILE) != 0 {
		t.Fatal("voice and round video crossed")
	}
}

func TestLegacyAudioCountsOnce(t *testing.T) {
	voice := voiceNote()
	round := roundVideo()
	rows := []visibleMedia{
		{FilterType: mtproto.MEDIA_AUDIO, Message: voice},
		{FilterType: mtproto.MEDIA_AUDIO, Message: round},
		{FilterType: mtproto.MEDIA_VOICE_FILE, Message: voice},
		{FilterType: mtproto.MEDIA_ROUND_FILE, Message: round},
	}
	if got := countVisibleMedia(rows, mtproto.MEDIA_VOICE_FILE); got != 2 {
		t.Fatalf("voice %d, want 2 (legacy plus new, no double count)", got)
	}
	if got := countVisibleMedia(rows, mtproto.MEDIA_ROUND_FILE); got != 2 {
		t.Fatalf("round %d, want 2", got)
	}
	if got := countVisibleMedia(rows, mtproto.MEDIA_AUDIO); got != 4 {
		t.Fatalf("round voice %d, want 4", got)
	}
	// A new voice row is not also a legacy MEDIA_AUDIO row.
	onlyNew := []visibleMedia{{FilterType: mtproto.MEDIA_VOICE_FILE, Message: voice}}
	if got := countVisibleMedia(onlyNew, mtproto.MEDIA_VOICE_FILE); got != 1 {
		t.Fatalf("new voice counted %d times", got)
	}
}

func TestSearchStillReadsLegacyAudio(t *testing.T) {
	voice := searchMediaTypes(mtproto.MEDIA_VOICE_FILE)
	if !containsInt32(voice, mtproto.MEDIA_VOICE_FILE) || !containsInt32(voice, mtproto.MEDIA_AUDIO) {
		t.Fatalf("voice search types %v", voice)
	}
	round := searchMediaTypes(mtproto.MEDIA_ROUND_FILE)
	if !containsInt32(round, mtproto.MEDIA_ROUND_FILE) || !containsInt32(round, mtproto.MEDIA_AUDIO) {
		t.Fatalf("round search types %v", round)
	}
	both := searchMediaTypes(mtproto.MEDIA_AUDIO)
	if !containsInt32(both, mtproto.MEDIA_AUDIO) || !containsInt32(both, mtproto.MEDIA_VOICE_FILE) || !containsInt32(both, mtproto.MEDIA_ROUND_FILE) {
		t.Fatalf("round-voice search types %v", both)
	}
	photo := searchMediaTypes(mtproto.MEDIA_PHOTOS_ONLY)
	if len(photo) != 1 || photo[0] != mtproto.MEDIA_PHOTOS_ONLY {
		t.Fatalf("photo search types %v", photo)
	}
}

func TestMergeNewestBoxesKeepsTheNewestPage(t *testing.T) {
	box := func(id int32) *mtproto.MessageBox {
		return &mtproto.MessageBox{MessageId: id}
	}
	got := mergeNewestBoxes([][]*mtproto.MessageBox{
		{box(10), box(8), box(6)},
		{box(9), box(8), nil},
		{box(7)},
	}, 3)
	if len(got) != 3 || got[0].MessageId != 10 || got[1].MessageId != 9 || got[2].MessageId != 8 {
		t.Fatalf("merged %#v", got)
	}
}

func containsInt32(list []int32, v int32) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}
