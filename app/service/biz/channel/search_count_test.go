package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestCountByFilterSplitsPhotoVideoVoiceAndRound(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Media", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	photo := mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia()
	video := mediaDoc("video/mp4", mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
		W: 320, H: 240, Duration: 2,
	}).To_DocumentAttribute())
	voice := mediaDoc("audio/ogg", mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{
		Voice: true, Duration: 1,
	}).To_DocumentAttribute())
	round := mediaDoc("video/mp4", mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
		RoundMessage: true, W: 240, H: 240, Duration: 1,
	}).To_DocumentAttribute())
	msg := func(text string, media *mtproto.MessageMedia) *mtproto.Message {
		return mtproto.MakeTLMessage(&mtproto.Message{Message: text, Media: media}).To_Message()
	}
	if _, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{
		{RandomID: 1, Message: msg("hello", nil)},
		{RandomID: 2, Message: msg("pic", photo)},
		{RandomID: 3, Message: msg("clip", video)},
		{RandomID: 4, Message: msg("note", voice)},
		{RandomID: 5, Message: msg("bubble", round)},
	}); err != nil {
		t.Fatal(err)
	}
	want := []struct {
		name   string
		filter *mtproto.MessagesFilter
		count  int32
	}{
		{"photos", mtproto.MakeTLInputMessagesFilterPhotos(&mtproto.MessagesFilter{}).To_MessagesFilter(), 1},
		{"video", mtproto.MakeTLInputMessagesFilterVideo(&mtproto.MessagesFilter{}).To_MessagesFilter(), 1},
		{"photoVideo", mtproto.MakeTLInputMessagesFilterPhotoVideo(&mtproto.MessagesFilter{}).To_MessagesFilter(), 2},
		{"voice", mtproto.MakeTLInputMessagesFilterVoice(&mtproto.MessagesFilter{}).To_MessagesFilter(), 1},
		{"round", mtproto.MakeTLInputMessagesFilterRoundVideo(&mtproto.MessagesFilter{}).To_MessagesFilter(), 1},
		{"roundVoice", mtproto.MakeTLInputMessagesFilterRoundVoice(&mtproto.MessagesFilter{}).To_MessagesFilter(), 2},
	}
	for _, tc := range want {
		n, err := svc.CountByFilter(ctx, 1, ch.Id, tc.filter)
		if err != nil || n != tc.count {
			t.Fatalf("%s count %d err %v, want %d", tc.name, n, err, tc.count)
		}
		found, err := svc.SearchInChannel(ctx, 1, ch.Id, ChannelSearch{Filter: tc.filter, Limit: 20})
		if err != nil || int32(len(found.GetMessages())) != tc.count {
			t.Fatalf("%s search %d err %v, want %d", tc.name, len(found.GetMessages()), err, tc.count)
		}
	}
}

func mediaDoc(mime string, attr *mtproto.DocumentAttribute) *mtproto.MessageMedia {
	doc := mtproto.MakeTLDocument(&mtproto.Document{
		Id:         1,
		MimeType:   mime,
		Attributes: []*mtproto.DocumentAttribute{attr},
	}).To_Document()
	return mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
		Document: doc,
	}).To_MessageMedia()
}
