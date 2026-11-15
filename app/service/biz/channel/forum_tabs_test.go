package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestToggleForumStoresTabsAndViewAsMessages(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Tabs", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ToggleForum(ctx, 2, in, true, true); err == nil {
		t.Fatal("member toggled forum")
	}
	if _, err = svc.ToggleForum(ctx, 1, in, true, true); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetChannels(ctx, 1, []*mtproto.InputChannel{in})
	if err != nil {
		t.Fatal(err)
	}
	chat := got.GetChats()[0]
	if !chat.GetForum() || !chat.GetForumTabs() {
		t.Fatalf("forum=%v tabs=%v", chat.GetForum(), chat.GetForumTabs())
	}
	if _, err = svc.ToggleViewForumAsMessages(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.DialogsFor(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var viewed bool
	for _, row := range rows {
		if row.ChannelID == ch.Id && row.ViewForumAsMessages {
			viewed = true
		}
	}
	if !viewed {
		t.Fatalf("view forum as messages %#v", rows)
	}
	if _, err = svc.ToggleForum(ctx, 1, in, false, true); err != nil {
		t.Fatal(err)
	}
	got, err = svc.GetChannels(ctx, 1, []*mtproto.InputChannel{in})
	if err != nil {
		t.Fatal(err)
	}
	chat = got.GetChats()[0]
	if chat.GetForum() || chat.GetForumTabs() {
		t.Fatalf("after off forum=%v tabs=%v", chat.GetForum(), chat.GetForumTabs())
	}
}
