package channel

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestRepliesDeliveryPrivacy(t *testing.T) {
	open := RepliesDeliveryOf(false, 100, nil)
	if !open.Write || !open.Push || open.Silent {
		t.Fatalf("default %#v", open)
	}
	if got := RepliesDeliveryOf(true, 100, nil); got.Write || got.Push {
		t.Fatalf("blocked %#v", got)
	}
	hidden := RepliesDeliveryOf(false, 100, mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		ShowPreviews: mtproto.ToBool(false),
	}).To_PeerNotifySettings())
	if !hidden.Write || !hidden.Push || hidden.Silent {
		t.Fatalf("preview off still delivers %#v", hidden)
	}
	muted := RepliesDeliveryOf(false, 100, mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		MuteUntil: wrapperspb.Int32(200),
		Silent:    mtproto.ToBool(true),
	}).To_PeerNotifySettings())
	if !muted.Write || !muted.Push || !muted.Silent {
		t.Fatalf("muted %#v", muted)
	}
	past := RepliesDeliveryOf(false, 100, mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		MuteUntil: wrapperspb.Int32(50),
	}).To_PeerNotifySettings())
	if !past.Write || !past.Push || past.Silent {
		t.Fatalf("elapsed mute %#v", past)
	}
}

func TestDiscussionReplyWithoutThreadIndexDoesNotNotify(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "")
	svc, ctx := newTest(t)
	group := linkedTalk(t, svc, ctx)
	box := newNoticeBox()
	svc.SetHooks(Hooks{RepliesInbox: box.keep})
	if _, err := svc.Invite(ctx, 1, 0, inCh(group), []int64{2, 3, 4}, nil); err != nil {
		t.Fatal(err)
	}
	parent, err := svc.Send(ctx, 2, group.Id, 0, []Outgoing{{
		RandomID: 11,
		Message:  &mtproto.Message{Message: "parent-note", Date: 20},
	}})
	if err != nil {
		t.Fatal(err)
	}
	parentID := sentMessageID(parent)
	reply, err := svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 12,
		Message:  replyMessage("reply-note", 21, parentID),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if sentMessageID(reply) == 0 || len(box.notes) != 0 || box.calls != 0 {
		t.Fatalf("id %d notices %d calls %d", sentMessageID(reply), len(box.notes), box.calls)
	}
}

func TestRepliesNoticeSkipsUnlinkedAndSelf(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "")
	svc, ctx := newTest(t)
	box := newNoticeBox()
	svc.SetHooks(Hooks{RepliesInbox: box.keep})
	room := megagroup(t, svc, ctx, "Plain")
	if _, err := svc.Invite(ctx, 1, 0, inCh(room), []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	parent, err := svc.Send(ctx, 2, room.Id, 0, []Outgoing{{
		RandomID: 1,
		Message:  &mtproto.Message{Message: "p", Date: 5},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 3, room.Id, 0, []Outgoing{{
		RandomID: 2,
		Message:  replyMessage("r", 6, sentMessageID(parent)),
	}}); err != nil {
		t.Fatal(err)
	}
	group := linkedTalk(t, svc, ctx)
	if _, err = svc.Invite(ctx, 1, 0, inCh(group), []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	own, err := svc.Send(ctx, 2, group.Id, 0, []Outgoing{{
		RandomID: 3,
		Message:  &mtproto.Message{Message: "mine", Date: 7},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 2, group.Id, 0, []Outgoing{{
		RandomID: 4,
		Message:  replyMessage("self", 8, sentMessageID(own)),
	}}); err != nil {
		t.Fatal(err)
	}
	if len(box.notes) != 0 {
		t.Fatalf("notices %+v", box.notes)
	}
}

func TestRepliesNoticeHidesFromUnreadAndRestricted(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "1")
	svc, ctx := newTest(t)
	group, commentID := threadComment(t, svc, ctx)
	box := newNoticeBox()
	svc.SetHooks(Hooks{RepliesInbox: box.keep})
	in := inCh(group)
	p, err := svc.store.GetParticipant(ctx, group.Id, 2)
	if err != nil || p == nil {
		t.Fatal(err)
	}
	p.Date = 1000
	if err = svc.store.PutParticipant(ctx, p); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.TogglePreHistory(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 22,
		Message:  replyMessage("after-join", 30, commentID),
	}}); err != nil || len(box.notes) != 0 {
		t.Fatalf("prehistory notices %d %v", len(box.notes), err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{BanUsers: true}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, 1, 0, in, 2, rights, "mod"); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 23,
		Message:  replyMessage("admin-sees", 31, commentID),
	}}); err != nil || len(box.notes) != 1 || box.notes[0].Recipient != 2 {
		t.Fatalf("admin notices %d %v", len(box.notes), err)
	}
	box.notes = nil
	viewBan := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		ViewMessages: true,
		UntilDate:    2147483646,
	}).To_ChatBannedRights()
	if _, err = svc.EditBanned(ctx, 1, 0, in, 2, viewBan); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 24,
		Message:  replyMessage("banned-view", 32, commentID),
	}}); err != nil || len(box.notes) != 0 {
		t.Fatalf("view ban notices %d %v", len(box.notes), err)
	}
	raw, err := svc.store.GetChannel(ctx, group.Id)
	if err != nil {
		t.Fatal(err)
	}
	raw.Restricted = true
	if err = svc.store.SaveChannel(ctx, raw); err != nil {
		t.Fatal(err)
	}
	open := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights()
	if _, err = svc.EditBanned(ctx, 1, 0, in, 2, open); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 25,
		Message:  replyMessage("restricted-body", 33, commentID),
	}}); err != nil || len(box.notes) != 0 {
		t.Fatalf("restricted notices %d %v", len(box.notes), err)
	}
}

func TestRepliesNoticeLeftMemberDoesNotReceiveViaPublicChannel(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "1")
	svc, ctx := newTest(t)
	group, news, commentID := threadCommentOn(t, svc, ctx)
	box := newNoticeBox()
	svc.SetHooks(Hooks{RepliesInbox: box.keep})
	if _, err := svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 31,
		Message:  replyMessage("while-member", 40, commentID),
	}}); err != nil || len(box.notes) != 1 || box.notes[0].Message.GetMessage() != "while-member" {
		t.Fatalf("member notices %+v %v", box.notes, err)
	}
	if _, err := svc.Leave(ctx, 2, 0, inCh(group)); err != nil {
		t.Fatal(err)
	}
	box.notes = nil
	if _, err := svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 32,
		Message:  replyMessage("after-leave", 41, commentID),
	}}); err != nil || len(box.notes) != 0 {
		t.Fatalf("left notices %d %v", len(box.notes), err)
	}
	if _, err := svc.UpdateUsername(ctx, 1, 0, inCh(news), "newsroom"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 33,
		Message:  replyMessage("public-channel", 42, commentID),
	}}); err != nil || len(box.notes) != 0 {
		t.Fatalf("public channel bypass %d %v", len(box.notes), err)
	}
}

func TestRepliesNoticeBlockedPreviewAndRetry(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "1")
	svc, ctx := newTest(t)
	group, commentID := threadComment(t, svc, ctx)
	var calls int
	var stored []RepliesNotice
	var pushed []int64
	blocked := map[int64]bool{}
	var settings []*mtproto.PeerNotifySettings
	failLeft := 1
	svc.SetHooks(Hooks{RepliesInbox: func(_ context.Context, n RepliesNotice) error {
		calls++
		if failLeft > 0 {
			failLeft--
			return errors.New("inbox down")
		}
		pol := RepliesDeliveryOf(blocked[n.Recipient], 100, settings...)
		if !pol.Write {
			return nil
		}
		if pol.Silent && n.Message != nil {
			n.Message.Silent = true
		}
		if pol.Push {
			pushed = append(pushed, n.Recipient)
		}
		for _, prev := range stored {
			if prev.RandomID == n.RandomID {
				return nil
			}
		}
		stored = append(stored, n)
		return nil
	}})
	if _, err := svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 42,
		Message:  replyMessage("first", 41, commentID),
	}}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(stored) != 1 || len(pushed) != 1 {
		t.Fatalf("calls %d stored %d pushed %d", calls, len(stored), len(pushed))
	}
	blocked[2] = true
	if _, err := svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 43,
		Message:  replyMessage("after-block", 42, commentID),
	}}); err != nil || len(stored) != 1 {
		t.Fatalf("after block stored %d %v", len(stored), err)
	}
	blocked[2] = false
	settings = []*mtproto.PeerNotifySettings{mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		ShowPreviews: mtproto.ToBool(false),
	}).To_PeerNotifySettings()}
	pushed = nil
	if _, err := svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 44,
		Message:  replyMessage("quiet", 43, commentID),
	}}); err != nil || len(stored) != 2 || stored[1].Message.GetSilent() || len(pushed) != 1 || pushed[0] != 2 {
		silent := false
		if len(stored) > 1 && stored[1].Message != nil {
			silent = stored[1].Message.GetSilent()
		}
		t.Fatalf("preview stored %d silent %v pushed %#v %v", len(stored), silent, pushed, err)
	}
	if stored[1].Message.GetMessage() != "quiet" || stored[1].Commenter != 3 {
		t.Fatal("preview off dropped the replies chat row")
	}
}

func TestRepliesNoticeAnonymousAndRestrictedText(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "")
	svc, ctx := newTest(t)
	group := linkedTalk(t, svc, ctx)
	box := newNoticeBox()
	svc.SetHooks(Hooks{RepliesInbox: box.keep})
	if _, err := svc.Invite(ctx, 1, 0, inCh(group), []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	visible, err := svc.Send(ctx, 2, group.Id, 0, []Outgoing{{
		RandomID: 51,
		Message:  &mtproto.Message{Message: "visible", Date: 50},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 52,
		Message: &mtproto.Message{
			Message: "hidden-body",
			Date:    51,
			ReplyTo: replyHeader(sentMessageID(visible)),
			RestrictionReason: []*mtproto.RestrictionReason{{
				Platform: "all",
				Reason:   "porn",
				Text:     "hidden",
			}},
		},
	}}); err != nil || len(box.notes) != 0 {
		t.Fatalf("restricted text notices %d %v", len(box.notes), err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{Anonymous: true}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, 1, 0, inCh(group), 2, rights, "mod"); err != nil {
		t.Fatal(err)
	}
	anon, err := svc.Send(ctx, 2, group.Id, 0, []Outgoing{{
		RandomID: 53,
		Message:  &mtproto.Message{Message: "anon", Date: 52},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 54,
		Message:  replyMessage("to-anon", 53, sentMessageID(anon)),
	}}); err != nil || len(box.notes) != 0 {
		t.Fatalf("anon notices %d %v", len(box.notes), err)
	}
}

func TestRepliesNoticeCommentThreadAndGeneralChat(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "1")
	svc, ctx := newTest(t)
	group, news := linkedPair(t, svc, ctx)
	box := newNoticeBox()
	svc.SetHooks(Hooks{RepliesInbox: box.keep})
	if _, err := svc.Invite(ctx, 1, 0, inCh(group), []int64{2, 3, 4}, nil); err != nil {
		t.Fatal(err)
	}
	post, err := svc.Send(ctx, 1, news.Id, 0, []Outgoing{{
		RandomID: 61,
		Message:  &mtproto.Message{Message: "post", Date: 60},
	}})
	if err != nil {
		t.Fatal(err)
	}
	general, err := svc.Send(ctx, 2, group.Id, 0, []Outgoing{{
		RandomID: 62,
		Message:  &mtproto.Message{Message: "general", Date: 61},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 63,
		Message:  replyMessage("off-thread", 62, sentMessageID(general)),
	}}); err != nil || len(box.notes) != 0 {
		t.Fatalf("general chat notices %d %v", len(box.notes), err)
	}
	comment, err := svc.Send(ctx, 2, news.Id, 0, []Outgoing{{
		RandomID: 64,
		Message:  replyMessage("top-comment", 63, sentMessageID(post)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(box.notes) != 0 {
		t.Fatal("top-level comment notified a user for the channel post")
	}
	if _, err = svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 65,
		Message:  replyMessage("thread-reply", 64, sentMessageID(comment)),
	}}); err != nil || len(box.notes) != 1 {
		t.Fatalf("thread notices %d %v", len(box.notes), err)
	}
	n := box.notes[0]
	if n.Recipient != 2 || n.Commenter != 3 || n.Message.GetMessage() != "thread-reply" {
		t.Fatalf("thread notice %+v", n)
	}
	for _, id := range []int64{1, 4} {
		if n.Recipient == id {
			t.Fatalf("notified %d", id)
		}
	}
}

func TestRepliesNoticeHookFailureDoesNotDropTheComment(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "1")
	svc, ctx := newTest(t)
	group, commentID := threadComment(t, svc, ctx)
	var calls int
	svc.SetHooks(Hooks{RepliesInbox: func(context.Context, RepliesNotice) error {
		calls++
		return errors.New("inbox down")
	}})
	_, err := svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 72,
		Message:  replyMessage("kept", 71, commentID),
	}})
	if err == nil || calls != 3 {
		t.Fatalf("err %v calls %d", err, calls)
	}
	rows, err := svc.store.ListMessages(ctx, group.Id)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, row := range rows {
		if row == nil || row.Deleted {
			continue
		}
		body, uerr := unmarshalMessage(row.Data)
		if uerr == nil && body.GetMessage() == "kept" {
			found = true
		}
	}
	if !found {
		t.Fatal("failed notice deleted the comment")
	}
	box := newNoticeBox()
	svc.SetHooks(Hooks{RepliesInbox: box.keep})
	again, err := svc.Send(ctx, 3, group.Id, 0, []Outgoing{{
		RandomID: 72,
		Message:  replyMessage("kept", 71, commentID),
	}})
	if err != nil || sentMessageID(again) == 0 || len(box.notes) != 1 {
		t.Fatalf("retry notices %d id %d %v", len(box.notes), sentMessageID(again), err)
	}
	if box.notes[0].Message.GetMessage() != "kept" || box.notes[0].Recipient != 2 {
		t.Fatalf("retry notice %+v", box.notes[0])
	}
}

type noticeBox struct {
	notes []RepliesNotice
	calls int
}

func newNoticeBox() *noticeBox { return &noticeBox{} }

func (b *noticeBox) keep(_ context.Context, n RepliesNotice) error {
	b.calls++
	for _, prev := range b.notes {
		if prev.RandomID == n.RandomID {
			return nil
		}
	}
	b.notes = append(b.notes, n)
	return nil
}

func threadComment(t *testing.T, svc *Service, ctx context.Context) (*mtproto.Chat, int32) {
	t.Helper()
	group, _, id := threadCommentOn(t, svc, ctx)
	return group, id
}

func threadCommentOn(t *testing.T, svc *Service, ctx context.Context) (group, news *mtproto.Chat, commentID int32) {
	t.Helper()
	group, news = linkedPair(t, svc, ctx)
	if _, err := svc.Invite(ctx, 1, 0, inCh(group), []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	post, err := svc.Send(ctx, 1, news.Id, 0, []Outgoing{{
		RandomID: 9001,
		Message:  &mtproto.Message{Message: "post", Date: 10},
	}})
	if err != nil {
		t.Fatal(err)
	}
	comment, err := svc.Send(ctx, 2, news.Id, 0, []Outgoing{{
		RandomID: 9002,
		Message:  replyMessage("top-comment", 11, sentMessageID(post)),
	}})
	if err != nil {
		t.Fatal(err)
	}
	commentID = sentMessageID(comment)
	if commentID == 0 {
		t.Fatal("comment id")
	}
	return group, news, commentID
}

func linkedTalk(t *testing.T, svc *Service, ctx context.Context) *mtproto.Chat {
	t.Helper()
	group, _ := linkedPair(t, svc, ctx)
	return group
}

func linkedPair(t *testing.T, svc *Service, ctx context.Context) (group, news *mtproto.Chat) {
	t.Helper()
	broadcast, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	room, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Talk", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	news = broadcast.Chats[0]
	group = room.Chats[0]
	if err = svc.SetDiscussionGroup(ctx, 1, 0, inCh(news), inCh(group)); err != nil {
		t.Fatal(err)
	}
	return group, news
}

func replyMessage(text string, date, parent int32) *mtproto.Message {
	return &mtproto.Message{Message: text, Date: date, ReplyTo: replyHeader(parent)}
}

func replyHeader(parent int32) *mtproto.MessageReplyHeader {
	return mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId:           parent,
		ReplyToMsgId_INT32:     parent,
		ReplyToMsgId_FLAGINT32: wrapperspb.Int32(parent),
	}).To_MessageReplyHeader()
}

func sentMessageID(up *mtproto.Updates) int32 {
	var id int32
	if up == nil {
		return 0
	}
	for _, u := range up.GetUpdates() {
		if m := u.GetMessage_MESSAGE(); m.GetId() > id {
			id = m.GetId()
		}
	}
	return id
}
