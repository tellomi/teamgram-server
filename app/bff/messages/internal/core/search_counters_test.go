package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSearchCounterMediaUsesStoredTypes(t *testing.T) {
	photo := mtproto.MakeTLInputMessagesFilterPhotos(&mtproto.MessagesFilter{}).To_MessagesFilter()
	video := mtproto.MakeTLInputMessagesFilterVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()
	both := mtproto.MakeTLInputMessagesFilterPhotoVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()
	voice := mtproto.MakeTLInputMessagesFilterVoice(&mtproto.MessagesFilter{}).To_MessagesFilter()
	round := mtproto.MakeTLInputMessagesFilterRoundVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()
	pair := mtproto.MakeTLInputMessagesFilterRoundVoice(&mtproto.MessagesFilter{}).To_MessagesFilter()
	cases := []struct {
		filter *mtproto.MessagesFilter
		want   int32
	}{
		{photo, mtproto.MEDIA_PHOTOS_ONLY},
		{video, mtproto.MEDIA_VIDEOS_ONLY},
		{both, mtproto.MEDIA_PHOTOVIDEO},
		{voice, mtproto.MEDIA_VOICE_FILE},
		{round, mtproto.MEDIA_ROUND_FILE},
		{pair, mtproto.MEDIA_AUDIO},
	}
	for _, tc := range cases {
		got, ok := searchCounterMedia(tc.filter)
		if !ok || got != tc.want {
			t.Fatalf("%s -> %d ok=%v, want %d", tc.filter.GetPredicateName(), got, ok, tc.want)
		}
	}
	if _, ok := searchCounterMedia(mtproto.MakeTLInputMessagesFilterMyMentions(&mtproto.MessagesFilter{}).To_MessagesFilter()); ok {
		t.Fatal("mentions are not a media counter")
	}
}
