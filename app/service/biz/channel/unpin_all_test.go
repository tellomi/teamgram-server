package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestUnpinAllClearsEveryPinnedMessage(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Pins", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	send := func(text string, random int64) int32 {
		t.Helper()
		sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
			RandomID: random,
			Message:  mtproto.MakeTLMessage(&mtproto.Message{Message: text}).To_Message(),
		}})
		if err != nil {
			t.Fatal(err)
		}
		return sent.Updates[0].GetMessage_MESSAGE().GetId()
	}
	first := send("one", 1)
	second := send("two", 2)
	if _, err := svc.PinMessage(ctx, 1, 0, ch.Id, first, false); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PinMessage(ctx, 1, 0, ch.Id, second, false); err != nil {
		t.Fatal(err)
	}
	pinnedFilter := mtproto.MakeTLInputMessagesFilterPinned(&mtproto.MessagesFilter{}).To_MessagesFilter()
	found, err := svc.SearchInChannel(ctx, 1, ch.Id, ChannelSearch{Filter: pinnedFilter, Limit: 20})
	if err != nil || len(found.GetMessages()) != 2 {
		t.Fatalf("two pins %#v %v", found.GetMessages(), err)
	}

	if _, err := svc.PinMessage(ctx, 1, 0, ch.Id, first, true); err != nil {
		t.Fatal(err)
	}
	found, err = svc.SearchInChannel(ctx, 1, ch.Id, ChannelSearch{Filter: pinnedFilter, Limit: 20})
	if err != nil || len(found.GetMessages()) != 1 || found.GetMessages()[0].GetId() != second {
		t.Fatalf("after unpin one %#v %v", found.GetMessages(), err)
	}

	pts, count, err := svc.UnpinAll(ctx, 1, 0, ch.Id)
	if err != nil || count != 1 || pts == 0 {
		t.Fatalf("unpin all pts=%d count=%d err=%v", pts, count, err)
	}
	found, err = svc.SearchInChannel(ctx, 1, ch.Id, ChannelSearch{Filter: pinnedFilter, Limit: 20})
	if err != nil || len(found.GetMessages()) != 0 {
		t.Fatalf("still pinned %#v %v", found.GetMessages(), err)
	}
	if _, count, err = svc.UnpinAll(ctx, 1, 0, ch.Id); err != nil || count != 0 {
		t.Fatalf("second unpin count=%d err=%v", count, err)
	}
}
