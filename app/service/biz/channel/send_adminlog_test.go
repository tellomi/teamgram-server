package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestSendReplyCarriesMessageID(t *testing.T) {
	svc, ctx := newTest(t)
	var pushed []*mtproto.Updates
	svc.SetHooks(Hooks{Push: func(_ context.Context, _ int64, _ []int64, updates *mtproto.Updates) {
		pushed = append(pushed, updates)
	}})
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	pushed = nil
	sent, err := svc.Send(ctx, 1, ch.Id, 9, []Outgoing{{
		RandomID: 77,
		Message:  &mtproto.Message{Message: "hi", Date: 3},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(sent.Updates) < 2 {
		t.Fatalf("updates %d", len(sent.Updates))
	}
	idUp := sent.Updates[0]
	msgUp := sent.Updates[1]
	if idUp.GetPredicateName() != mtproto.Predicate_updateMessageID {
		t.Fatalf("first update %s", idUp.GetPredicateName())
	}
	if msgUp.GetPredicateName() != mtproto.Predicate_updateNewChannelMessage {
		t.Fatalf("second update %s", msgUp.GetPredicateName())
	}
	if idUp.GetRandomId() != 77 || idUp.GetId_INT32() != msgUp.GetMessage_MESSAGE().GetId() || msgUp.GetMessage_MESSAGE().GetId() == 0 {
		t.Fatalf("message id update %#v msg %#v", idUp, msgUp.GetMessage_MESSAGE())
	}
	buf := mtproto.NewEncodeBuf(1024)
	if err := sent.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if len(pushed) == 0 {
		t.Fatal("members were not notified")
	}
	for _, up := range pushed {
		for _, u := range up.GetUpdates() {
			if u.GetPredicateName() == mtproto.Predicate_updateMessageID {
				t.Fatal("fanout included the sender random id")
			}
		}
	}

	again, err := svc.Send(ctx, 1, ch.Id, 9, []Outgoing{{
		RandomID: 77,
		Message:  &mtproto.Message{Message: "hi", Date: 3},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if again.Updates[0].GetPredicateName() != mtproto.Predicate_updateMessageID || again.Updates[0].GetId_INT32() != idUp.GetId_INT32() {
		t.Fatalf("replay %#v", again.Updates[0])
	}
}

func TestAdminLogPhotoEncodes(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	photo := mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:         42,
		AccessHash: 7,
		Date:       10,
		DcId:       1,
		Sizes: []*mtproto.PhotoSize{
			mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{Type: "a", W: 160, H: 160, Size2: 20}).To_PhotoSize(),
		},
	}).To_Photo()
	if _, err := svc.ApplyPhoto(ctx, 1, 0, in, photo.GetId(), photo); err != nil {
		t.Fatal(err)
	}
	// A row written before both photos were required. The reader must still
	// return instead of panicking in the TL encoder.
	svc.logAdmin(ctx, ch.Id, 1, mtproto.MakeTLChannelAdminLogEventActionChangePhoto(&mtproto.ChannelAdminLogEventAction{
		NewPhoto: photo,
	}).To_ChannelAdminLogEventAction())

	log, err := svc.GetAdminLog(ctx, 1, in, "", nil, nil, 0, 0, 20)
	if err != nil {
		t.Fatal(err)
	}
	var saw int
	for _, ev := range log.GetEvents() {
		if ev.GetAction().GetPredicateName() != mtproto.Predicate_channelAdminLogEventActionChangePhoto {
			continue
		}
		saw++
		if ev.GetAction().GetPrevPhoto() == nil || ev.GetAction().GetNewPhoto() == nil {
			t.Fatalf("nil photo %#v", ev.GetAction())
		}
	}
	if saw < 2 {
		t.Fatalf("changePhoto events %d", saw)
	}
	buf := mtproto.NewEncodeBuf(4096)
	if err := log.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
}
