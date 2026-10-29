package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func newTest(t *testing.T) (*Service, context.Context) {
	t.Helper()
	return New(NewMemoryStore()), context.Background()
}

func input(chID, hash int64) *mtproto.InputChannel {
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  chID,
		AccessHash: hash,
	}).To_InputChannel()
}

func TestCreateBroadcastAndSupergroup(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 10, Title: "News", Broadcast: true, About: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(up.Chats) != 1 || !up.Chats[0].Broadcast || up.Chats[0].Megagroup {
		t.Fatalf("broadcast chat = %#v", up.Chats[0])
	}
	if up.Chats[0].Id < mtproto.MinNebulaChatChannelID {
		t.Fatalf("channel id %d is in the basic-group range", up.Chats[0].Id)
	}
	if len(up.Updates) == 0 || up.Updates[0].GetPts_INT32() != 1 || up.Updates[0].GetPtsCount() != 1 {
		t.Fatalf("create updates = %#v", up.Updates)
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := up.Chats[0].Encode(buf, 232); err != nil {
		t.Fatal(err)
	}

	up2, err := svc.Create(ctx, CreateRequest{CreatorID: 10, Title: "Group", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	if !up2.Chats[0].Megagroup || !up2.Chats[0].Forum || up2.Chats[0].Id == up.Chats[0].Id {
		t.Fatalf("supergroup = %#v", up2.Chats[0])
	}
}

func TestMembershipInviteJoinLeave(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.AccessHash_FLAGINT64.Value)

	invited, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(invited.MissingInvitees) != 0 {
		t.Fatalf("missing = %#v", invited.MissingInvitees)
	}
	parts, err := svc.GetParticipants(ctx, 1, in, nil, 0, 100, 0)
	if err != nil {
		t.Fatal(err)
	}
	if parts.Count != 3 {
		t.Fatalf("participants = %d", parts.Count)
	}

	if _, err := svc.Join(ctx, 4, 0, in); err == nil {
		t.Fatal("private join should fail")
	}
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "room_one"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Join(ctx, 4, 0, in); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Leave(ctx, 4, 0, in); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Leave(ctx, 1, 0, in); err == nil {
		t.Fatal("creator leave should fail")
	}
	outsider, err := svc.GetChannels(ctx, 99, []*mtproto.InputChannel{in})
	if err != nil {
		t.Fatal(err)
	}
	if outsider.Chats[0].GetPredicateName() == mtproto.Predicate_channelForbidden {
		t.Fatal("public channel should be visible")
	}
	if !outsider.Chats[0].Left {
		t.Fatal("outsider should be marked left")
	}
}

func TestAdminBanAndRights(t *testing.T) {
	svc, ctx := newTest(t)
	created, _ := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Mods", Megagroup: true})
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	_, _ = svc.Invite(ctx, 1, 0, in, []int64{2}, nil)
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		DeleteMessages: true,
		BanUsers:       true,
		InviteUsers:    true,
	}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, rights, "mod"); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetParticipant(ctx, 1, in, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer())
	if err != nil {
		t.Fatal(err)
	}
	if got.Participant.GetPredicateName() != mtproto.Predicate_channelParticipantAdmin {
		t.Fatalf("participant = %s", got.Participant.GetPredicateName())
	}
	kick := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		ViewMessages: true,
		UntilDate:    int32(1<<31 - 1),
	}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 2, 0, in, 2, kick); err == nil {
		t.Fatal("admin must not kick self without being the target; kicking another admin")
	}
	_, _ = svc.Invite(ctx, 1, 0, in, []int64{3}, nil)
	if _, err := svc.EditBanned(ctx, 2, 0, in, 3, kick); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Join(ctx, 3, 0, input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())); err == nil {
		t.Fatal("kicked user rejoined")
	}
}

func TestMessagesPtsReadAndDifference(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Feed", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	_, _ = svc.Invite(ctx, 1, 0, in, []int64{2}, nil)
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "feednews"); err != nil {
		t.Fatal(err)
	}

	memberPost := &mtproto.Message{Message: "nope", Date: 10}
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 9, Message: memberPost}}); err == nil {
		t.Fatal("subscriber posted to a broadcast")
	}

	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 11,
		Message:  &mtproto.Message{Message: "hello", Date: 20},
	}})
	if err != nil {
		t.Fatal(err)
	}
	again, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 11,
		Message:  &mtproto.Message{Message: "hello", Date: 20},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if sent.Updates[0].Message_MESSAGE.Id != again.Updates[0].Message_MESSAGE.Id {
		t.Fatal("random id was not idempotent")
	}
	msgID := sent.Updates[0].Message_MESSAGE.Id
	if sent.Updates[0].Message_MESSAGE.Post == false || sent.Updates[0].Message_MESSAGE.FromId != nil {
		t.Fatalf("broadcast post flags = %#v", sent.Updates[0].Message_MESSAGE)
	}

	edited, err := svc.Edit(ctx, 1, ch.Id, 0, msgID, func(m *mtproto.Message) {
		m.Message = "hello!"
	})
	if err != nil {
		t.Fatal(err)
	}
	if edited.Updates[0].GetPredicateName() != mtproto.Predicate_updateEditChannelMessage {
		t.Fatalf("edit update = %s", edited.Updates[0].GetPredicateName())
	}

	other, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Other", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	if other.Chats[0].Id == ch.Id {
		t.Fatal("channels share an id")
	}
	if other.Updates[0].GetPts_INT32() != 1 {
		t.Fatalf("second channel pts = %d", other.Updates[0].GetPts_INT32())
	}

	full, err := svc.GetFullChannel(ctx, 2, in)
	if err != nil {
		t.Fatal(err)
	}
	if full.FullChat.UnreadCount < 1 {
		t.Fatalf("subscriber unread = %d", full.FullChat.UnreadCount)
	}
	if err := svc.ReadHistory(ctx, 2, in, msgID); err != nil {
		t.Fatal(err)
	}
	full, _ = svc.GetFullChannel(ctx, 2, in)
	if full.FullChat.UnreadCount != 0 || full.FullChat.ReadInboxMaxId < msgID {
		t.Fatalf("after read: unread %d inbox %d", full.FullChat.UnreadCount, full.FullChat.ReadInboxMaxId)
	}
	buf := mtproto.NewEncodeBuf(512)
	if err := full.FullChat.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}

	pts, count, err := svc.Delete(ctx, 1, ch.Id, 0, []int32{msgID})
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 || pts <= edited.Updates[0].GetPts_INT32() {
		t.Fatalf("delete pts=%d count=%d", pts, count)
	}

	diff, err := svc.Difference(ctx, 2, in, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	author, err := svc.Difference(ctx, 1, in, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range author.NewMessages {
		if m.Post && m.Message == "hello" && !m.Out {
			t.Fatal("author channel difference cleared out")
		}
	}
	if diff.GetPredicateName() != mtproto.Predicate_updates_channelDifference || !diff.Final || diff.Pts != pts {
		t.Fatalf("difference = %s final=%v pts=%d", diff.GetPredicateName(), diff.Final, diff.Pts)
	}
	if len(diff.NewMessages) < 2 {
		t.Fatalf("new messages = %d", len(diff.NewMessages))
	}
	empty, err := svc.Difference(ctx, 2, in, pts, 100)
	if err != nil {
		t.Fatal(err)
	}
	if empty.GetPredicateName() != mtproto.Predicate_updates_channelDifferenceEmpty || empty.Pts != pts {
		t.Fatalf("caught up = %s pts=%d", empty.GetPredicateName(), empty.Pts)
	}

	hist, err := svc.History(ctx, 2, in, HistoryQuery{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if hist.Count < 1 || hist.Pts != pts {
		t.Fatalf("history count=%d pts=%d", hist.Count, hist.Pts)
	}
	link, err := svc.ExportMessageLink(ctx, 1, in, 1)
	if err != nil {
		t.Fatal(err)
	}
	if link.Link == "" || link.Html == "" {
		t.Fatalf("link = %#v", link)
	}

	rows, err := svc.DialogsFor(ctx, 2)
	if err != nil || len(rows) != 1 || rows[0].ChannelID != ch.Id {
		t.Fatalf("dialogs = %#v %v", rows, err)
	}
}

func TestUsernameAndMigrate(t *testing.T) {
	svc, ctx := newTest(t)
	created, _ := svc.Create(ctx, CreateRequest{CreatorID: 7, Title: "Old", Megagroup: true})
	in := input(created.Chats[0].Id, created.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.CheckUsername(ctx, 7, in, "no"); err == nil {
		t.Fatal("short username accepted")
	}
	if _, err := svc.UpdateUsername(ctx, 7, 0, in, "taken_name"); err != nil {
		t.Fatal(err)
	}
	other, _ := svc.Create(ctx, CreateRequest{CreatorID: 8, Title: "Two", Broadcast: true})
	in2 := input(other.Chats[0].Id, other.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.UpdateUsername(ctx, 8, 0, in2, "taken_name"); err == nil {
		t.Fatal("duplicate username")
	}

	migrated, err := svc.Migrate(ctx, MigrateRequest{
		Actor:     7,
		ChatID:    42,
		Title:     "Old",
		CreatorID: 7,
		Members:   []LegacyMember{{UserID: 8}, {UserID: 9, IsBot: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if migrated.ChannelID < mtproto.MinNebulaChatChannelID || migrated.Already {
		t.Fatalf("migrate = %#v", migrated)
	}
	again, err := svc.Migrate(ctx, MigrateRequest{Actor: 7, ChatID: 42, Title: "Old", CreatorID: 7})
	if err != nil || !again.Already || again.ChannelID != migrated.ChannelID {
		t.Fatalf("second migrate = %#v %v", again, err)
	}
	full, err := svc.GetFullChannel(ctx, 7, input(migrated.ChannelID, migrated.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	if full.FullChat.GetMigratedFromChatId().GetValue() != 42 {
		t.Fatalf("migrated from = %d", full.FullChat.GetMigratedFromChatId().GetValue())
	}
	parts, err := svc.GetParticipants(ctx, 7, input(migrated.ChannelID, migrated.AccessHash), nil, 0, 50, 0)
	if err != nil || parts.Count != 3 {
		t.Fatalf("migrated participants = %#v %v", parts, err)
	}
}

func TestEditTitleEncodes(t *testing.T) {
	svc, ctx := newTest(t)
	created, _ := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "A", Megagroup: true})
	in := input(created.Chats[0].Id, created.Chats[0].GetAccessHash_FLAGINT64().GetValue())
	up, err := svc.EditTitle(ctx, 1, 0, in, "B")
	if err != nil {
		t.Fatal(err)
	}
	buf := mtproto.NewEncodeBuf(1024)
	if err := up.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	_ = wrapperspb.Int32(0)
}

func TestGroupOutboxWaitsForAnotherMember(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Three", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 21,
		Message:  &mtproto.Message{Message: "hello", Date: 30},
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := messageID(sent)
	if msgID == 0 {
		t.Fatal("sent message has no id")
	}

	sender := dialogRead(t, svc, ctx, 1, ch.Id)
	if sender.ReadOutbox >= msgID {
		t.Fatalf("sender outbox %d marked read before anyone else read %d", sender.ReadOutbox, msgID)
	}
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	if full.FullChat.ReadOutboxMaxId >= msgID {
		t.Fatalf("getFullChannel outbox %d", full.FullChat.ReadOutboxMaxId)
	}

	// The sender's own readHistory (what iOS sends while the chat is open) is not a receipt.
	if err := svc.ReadHistory(ctx, 1, in, msgID); err != nil {
		t.Fatal(err)
	}
	sender = dialogRead(t, svc, ctx, 1, ch.Id)
	if sender.ReadOutbox >= msgID {
		t.Fatalf("sender read their own message: outbox %d", sender.ReadOutbox)
	}

	if err := svc.ReadHistory(ctx, 2, in, msgID); err != nil {
		t.Fatal(err)
	}
	sender = dialogRead(t, svc, ctx, 1, ch.Id)
	if sender.ReadOutbox < msgID {
		t.Fatalf("after member 2 read, sender outbox %d want >= %d", sender.ReadOutbox, msgID)
	}
	reader := dialogRead(t, svc, ctx, 2, ch.Id)
	if reader.ReadInbox < msgID {
		t.Fatalf("reader inbox %d", reader.ReadInbox)
	}

	// Member 2's own send stays unread until member 3 reads it.
	sent, err = svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{
		RandomID: 22,
		Message:  &mtproto.Message{Message: "second", Date: 40},
	}})
	if err != nil {
		t.Fatal(err)
	}
	second := messageID(sent)
	if second <= msgID {
		t.Fatalf("second id %d", second)
	}
	if err := svc.ReadHistory(ctx, 2, in, second); err != nil {
		t.Fatal(err)
	}
	reader = dialogRead(t, svc, ctx, 2, ch.Id)
	if reader.ReadOutbox >= second {
		t.Fatalf("member 2 outbox %d marked read by their own read", reader.ReadOutbox)
	}
	if err := svc.ReadHistory(ctx, 3, in, second); err != nil {
		t.Fatal(err)
	}
	reader = dialogRead(t, svc, ctx, 2, ch.Id)
	if reader.ReadOutbox < second {
		t.Fatalf("after member 3 read, member 2 outbox %d want >= %d", reader.ReadOutbox, second)
	}
}

func messageID(up *mtproto.Updates) int32 {
	if up == nil {
		return 0
	}
	for _, u := range up.Updates {
		if u.Message_MESSAGE != nil && u.Message_MESSAGE.Id != 0 {
			return u.Message_MESSAGE.Id
		}
	}
	return 0
}

func dialogRead(t *testing.T, svc *Service, ctx context.Context, user, channelID int64) DialogRow {
	t.Helper()
	rows, err := svc.DialogsFor(ctx, user)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		if row.ChannelID == channelID {
			return row
		}
	}
	t.Fatalf("no dialog for user %d channel %d", user, channelID)
	return DialogRow{}
}
