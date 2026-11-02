package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSearchFiltersHaveStoredMediaTypes(t *testing.T) {
	msg := func(media *mtproto.MessageMedia) *mtproto.Message {
		return mtproto.MakeTLMessage(&mtproto.Message{Media: media}).To_Message()
	}
	cases := []struct {
		media *mtproto.MessageMedia
		want  int32
	}{
		{mtproto.MakeTLMessageMediaPoll(nil).To_MessageMedia(), mtproto.MEDIA_POLL},
		{mtproto.MakeTLMessageMediaGeo(nil).To_MessageMedia(), mtproto.MEDIA_GEO},
		{mtproto.MakeTLMessageMediaVenue(nil).To_MessageMedia(), mtproto.MEDIA_GEO},
		{mtproto.MakeTLMessageMediaGeoLive(nil).To_MessageMedia(), mtproto.MEDIA_GEO},
		{mtproto.MakeTLMessageMediaContact(nil).To_MessageMedia(), mtproto.MEDIA_CONTACT},
	}
	for _, tc := range cases {
		if got := mtproto.GetMediaType(msg(tc.media)); got != tc.want {
			t.Errorf("%s: media type %d, want %d", tc.media.GetPredicateName(), got, tc.want)
		}
	}
	poll := mtproto.MakeTLInputMessagesFilterPoll(nil).To_MessagesFilter()
	if mtproto.FromMessagesFilter(poll) != mtproto.FilterPoll {
		t.Fatal("inputMessagesFilterPoll is not FilterPoll")
	}
}

func TestKeepBoxes(t *testing.T) {
	a := &mtproto.MessageBox{MessageId: 1, Message: mtproto.MakeTLMessage(&mtproto.Message{Id: 1}).To_Message()}
	b := &mtproto.MessageBox{MessageId: 2, Message: mtproto.MakeTLMessage(&mtproto.Message{Id: 2}).To_Message()}
	got := keepBoxes(&mtproto.MessageBoxList{BoxList: []*mtproto.MessageBox{a, nil, b}}, func(m *mtproto.Message) bool {
		return m.GetId() == 2
	})
	if len(got.BoxList) != 1 || got.BoxList[0].MessageId != 2 {
		t.Fatalf("kept %v", got.BoxList)
	}
	if keepBoxes(nil, nil) != nil {
		t.Fatal("nil list")
	}
}
