package mtproto

import (
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func i32(v int32) *wrapperspb.Int32Value {
	return &wrapperspb.Int32Value{Value: v}
}

func TestCheckHasMediaUnreadTTL(t *testing.T) {
	photo := &Message{Media: MakeTLMessageMediaPhoto(&MessageMedia{
		Photo_FLAGPHOTO: MakeTLPhoto(&Photo{Id: 9}).To_Photo(),
	}).To_MessageMedia()}
	if CheckHasMediaUnread(photo) {
		t.Fatal("plain photo is not media_unread")
	}
	photo.Media.TtlSeconds = i32(ViewOnceTTL)
	if !CheckHasMediaUnread(photo) {
		t.Fatal("view-once photo must be media_unread")
	}
	timed := &Message{Media: MakeTLMessageMediaDocument(&MessageMedia{
		Document:   MakeTLDocument(&Document{Id: 3}).To_Document(),
		TtlSeconds: i32(10),
	}).To_MessageMedia()}
	if !CheckHasMediaUnread(timed) {
		t.Fatal("timed document must be media_unread")
	}
	voice := &Message{Media: MakeTLMessageMediaDocument(&MessageMedia{
		Document: MakeTLDocument(&Document{
			Id: 4,
			Attributes: []*DocumentAttribute{
				MakeTLDocumentAttributeAudio(&DocumentAttribute{Voice: true}).To_DocumentAttribute(),
			},
		}).To_Document(),
	}).To_MessageMedia()}
	if !CheckHasMediaUnread(voice) {
		t.Fatal("voice stays media_unread")
	}
}

func TestTTLDue(t *testing.T) {
	now := int32(1_700_000_000)
	if TTLDue(ViewOnceTTL, 0, now) {
		t.Fatal("unopened view-once is not due")
	}
	if !TTLDue(ViewOnceTTL, now, now) {
		t.Fatal("opened view-once is due immediately")
	}
	if TTLDue(10, now, now+9) {
		t.Fatal("timed media is still inside the open window")
	}
	if !TTLDue(10, now, now+10) {
		t.Fatal("timed media is due at open+ttl")
	}
	if TTLDue(10, 0, now+100) {
		t.Fatal("unopened timed media is not due")
	}
}

func TestStripTTLMedia(t *testing.T) {
	msg := &Message{MediaUnread: true, Media: MakeTLMessageMediaPhoto(&MessageMedia{
		LivePhoto:          true,
		Photo_FLAGPHOTO:    MakeTLPhoto(&Photo{Id: 42}).To_Photo(),
		TtlSeconds:         i32(ViewOnceTTL),
		Video_FLAGDOCUMENT: MakeTLDocument(&Document{Id: 7}).To_Document(),
	}).To_MessageMedia()}
	removed := StripTTLMedia(msg)
	if len(removed) != 2 || removed[0].ID != 42 || removed[1].ID != 7 {
		t.Fatalf("removed = %+v", removed)
	}
	if msg.GetMedia().GetTtlSeconds().GetValue() != ViewOnceTTL {
		t.Fatal("ttl_seconds was dropped")
	}
	if msg.GetMedia().GetPhoto_FLAGPHOTO().GetPredicateName() != Predicate_photoEmpty {
		t.Fatalf("photo = %s", msg.GetMedia().GetPhoto_FLAGPHOTO().GetPredicateName())
	}
	if msg.GetMedia().GetVideo_FLAGDOCUMENT() != nil {
		t.Fatal("live video was kept")
	}
	if msg.GetMedia().GetLivePhoto() {
		// live_photo and video share flags.4: the flag alone would encode
		// the bit with no Document.
		t.Fatal("expired live photo kept live_photo without its video")
	}
	if msg.GetMediaUnread() {
		t.Fatal("stripped message stays media_unread")
	}
	if again := StripTTLMedia(msg); len(again) != 0 {
		t.Fatalf("second strip removed %+v", again)
	}

	voice := &Message{Media: MakeTLMessageMediaDocument(&MessageMedia{
		TtlSeconds: i32(10),
		Document: MakeTLDocument(&Document{
			Id: 5,
			Attributes: []*DocumentAttribute{
				MakeTLDocumentAttributeAudio(&DocumentAttribute{Voice: true}).To_DocumentAttribute(),
			},
		}).To_Document(),
	}).To_MessageMedia()}
	StripTTLMedia(voice)
	if !voice.GetMedia().GetVoice() {
		t.Fatal("expired voice lost the voice flag")
	}
	if voice.GetMedia().GetDocument().GetPredicateName() != Predicate_documentEmpty {
		t.Fatal("voice document was not emptied")
	}
	plain := &Message{Media: MakeTLMessageMediaPhoto(&MessageMedia{
		Photo_FLAGPHOTO: MakeTLPhoto(&Photo{Id: 1}).To_Photo(),
	}).To_MessageMedia()}
	if StripTTLMedia(plain) != nil || plain.GetMedia().GetPhoto_FLAGPHOTO().GetId() != 1 {
		t.Fatal("plain photo was stripped")
	}
}

func TestMakeUpdateReadMessagesContentsDate(t *testing.T) {
	upd := MakeUpdateReadMessagesContents([]int32{4, 5}, 10, 2, 1_700_000_100)
	if upd.GetDate_FLAGINT32().GetValue() != 1_700_000_100 {
		t.Fatalf("date = %d", upd.GetDate_FLAGINT32().GetValue())
	}
	if len(upd.GetMessages()) != 2 || upd.GetPts_INT32() != 10 || upd.GetPtsCount() != 2 {
		t.Fatalf("update = %+v", upd)
	}
	if MakeUpdateReadMessagesContents(nil, 1, 1, 0).GetDate_FLAGINT32() != nil {
		t.Fatal("date 0 must stay unset")
	}
}
