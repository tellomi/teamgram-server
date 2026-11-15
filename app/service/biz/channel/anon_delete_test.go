package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/searchorder"
)

func TestAnonymousAdminFromID(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Anon", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		DeleteMessages: true,
		Anonymous:      true,
	}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, 1, 0, in, 2, rights, ""); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{
		RandomID: 21,
		Message:  &mtproto.Message{Message: "hidden", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	own := messageByText(sent.Updates, "hidden")
	if own == nil || !own.GetOut() || own.GetFromId().GetChannelId() != ch.Id || own.GetFromId().GetUserId() != 0 {
		t.Fatalf("sender copy %+v", own.GetFromId())
	}
	hist, err := svc.History(ctx, 3, in, HistoryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	seen := messageByText(messagesAsUpdates(hist.GetMessages()), "hidden")
	if seen == nil || seen.GetOut() || seen.GetFromId().GetChannelId() != ch.Id {
		t.Fatalf("member copy out=%v from %+v", seen.GetOut(), seen.GetFromId())
	}
	plain, err := svc.Send(ctx, 3, ch.Id, 0, []Outgoing{{
		RandomID: 22,
		Message:  &mtproto.Message{Message: "visible", Date: 11},
	}})
	if err != nil {
		t.Fatal(err)
	}
	vis := messageByText(plain.Updates, "visible")
	if vis == nil || vis.GetFromId().GetUserId() != 3 {
		t.Fatalf("member from %+v", vis.GetFromId())
	}

	found, err := svc.SearchPosts(ctx, 3, PostQuery{Needle: "hidden", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	hit := messageByText(messagesAsUpdates(found.GetMessages()), "hidden")
	if hit == nil || hit.GetFromId().GetChannelId() != ch.Id || hit.GetFromId().GetUserId() != 0 {
		t.Fatalf("searchPosts from %+v", hit.GetFromId())
	}
	for _, user := range found.GetUsers() {
		if user.GetId() == 2 {
			t.Fatal("searchPosts users include the anonymous admin")
		}
	}
	boxes, err := svc.SearchJoined(ctx, 3, "hidden", 10, searchorder.Key{}, false)
	if err != nil || len(boxes) == 0 {
		t.Fatalf("searchGlobal boxes %v %v", boxes, err)
	}
	shown := boxes[0].ToMessage(3)
	if shown.GetFromId().GetChannelId() != ch.Id || shown.GetFromId().GetUserId() != 0 {
		t.Fatalf("searchGlobal from %+v", shown.GetFromId())
	}
	byAuthor, err := svc.SearchInChannel(ctx, 3, ch.Id, ChannelSearch{FromUser: 2, Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if messageByText(messagesAsUpdates(byAuthor.GetMessages()), "hidden") != nil {
		t.Fatal("messages.search from the admin listed the anonymous post")
	}
	open, err := svc.SearchInChannel(ctx, 3, ch.Id, ChannelSearch{Q: "hidden", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	listed := messageByText(messagesAsUpdates(open.GetMessages()), "hidden")
	if listed == nil || listed.GetFromId().GetChannelId() != ch.Id {
		t.Fatalf("channel search from %+v", listed.GetFromId())
	}

	pinRights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		DeleteMessages: true,
		PinMessages:    true,
		Anonymous:      true,
	}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, 1, 0, in, 2, pinRights, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.PinMessage(ctx, 2, 0, ch.Id, own.GetId(), false); err != nil {
		t.Fatal(err)
	}
	after, err := svc.History(ctx, 3, in, HistoryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	var serviceKept bool
	for _, m := range after.GetMessages() {
		if m.GetAction().GetPredicateName() != mtproto.Predicate_messageActionPinMessage {
			continue
		}
		serviceKept = true
		if m.GetFromId().GetChannelId() != ch.Id || m.GetFromId().GetUserId() != 0 {
			t.Fatalf("service from %+v", m.GetFromId())
		}
	}
	if !serviceKept {
		t.Fatal("pin service message missing")
	}
}

func TestDeleteChannelPushesForbidden(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Gone", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	var pushed []int64
	var dropped []int64
	var sawForbidden, sawUpdate bool
	svc.SetHooks(Hooks{
		Push: func(_ context.Context, userID int64, _ []int64, updates *mtproto.Updates) {
			pushed = append(pushed, userID)
			for _, chat := range updates.GetChats() {
				if chat.GetPredicateName() == mtproto.Predicate_channelForbidden && chat.GetId() == ch.Id {
					sawForbidden = true
				}
			}
			for _, up := range updates.GetUpdates() {
				if up.GetPredicateName() == mtproto.Predicate_updateChannel && up.GetChannelId() == ch.Id {
					sawUpdate = true
				}
			}
		},
		DropDialog: func(_ context.Context, userID, channelID int64) {
			if channelID == ch.Id {
				dropped = append(dropped, userID)
			}
		},
	})
	if _, err = svc.DeleteChannel(ctx, 1, 99, in); err != nil {
		t.Fatal(err)
	}
	if !sawForbidden || !sawUpdate || !containsID(pushed, 1) || !containsID(pushed, 2) {
		t.Fatalf("push forbidden=%v update=%v users=%v", sawForbidden, sawUpdate, pushed)
	}
	if !containsID(dropped, 1) || !containsID(dropped, 2) {
		t.Fatalf("dialogs %v", dropped)
	}
	view, err := svc.GetChannels(ctx, 2, []*mtproto.InputChannel{in})
	if err != nil || view.Chats[0].GetPredicateName() != mtproto.Predicate_channelForbidden {
		t.Fatalf("after delete %#v %v", view, err)
	}
}

func messageByText(ups []*mtproto.Update, text string) *mtproto.Message {
	for _, up := range ups {
		if m := up.GetMessage_MESSAGE(); m != nil && m.GetMessage() == text {
			return m
		}
	}
	return nil
}

func messagesAsUpdates(msgs []*mtproto.Message) []*mtproto.Update {
	out := make([]*mtproto.Update, 0, len(msgs))
	for _, m := range msgs {
		out = append(out, &mtproto.Update{Message_MESSAGE: m})
	}
	return out
}

func containsID(ids []int64, id int64) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}
