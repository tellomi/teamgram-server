package channel

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// Synthetic users only. This stays inside the channel service memory store:
// no MySQL, no Kafka, no phone session.
const (
	histOwner  int64 = 910166001
	histOlder  int64 = 910166002
	histNewbie int64 = 910166003
)

func TestHiddenPrehistoryHidesBodiesAndReadList(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: histOwner, Title: "Hidden", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	if _, err = svc.Invite(ctx, histOwner, 0, in, []int64{histOlder}, nil); err != nil {
		t.Fatal(err)
	}

	oldDate := int32(time.Now().Unix()) - 3600
	sent, err := svc.Send(ctx, histOwner, ch.Id, 0, []Outgoing{{RandomID: 910166011, Message: &mtproto.Message{
		Message: "old-secret-166",
		Date:    oldDate,
		Media:   mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia(),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	oldID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	older, err := svc.store.GetParticipant(ctx, ch.Id, histOlder)
	if err != nil || older == nil {
		t.Fatal(err)
	}
	// The message date is an hour ago. The earlier member's join has to be
	// before that, or the same cutoff that hides prehistory would hide it
	// from them too. The new member joins after this, at the real clock.
	older.Date = oldDate - 60
	if err = svc.store.PutParticipant(ctx, older); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.PinMessage(ctx, histOwner, 0, ch.Id, oldID, false); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.TogglePreHistory(ctx, histOwner, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, histOwner, 0, in, []int64{histNewbie}, nil); err != nil {
		t.Fatal(err)
	}
	fresh, err := svc.Send(ctx, histOwner, ch.Id, 0, []Outgoing{{RandomID: 910166012, Message: &mtproto.Message{
		Message: "new-open-166",
		Date:    int32(time.Now().Unix()) + 30,
		ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{ReplyToMsgId: oldID}).To_MessageReplyHeader(),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	freshID := fresh.Updates[0].GetMessage_MESSAGE().GetId()

	hist, err := svc.History(ctx, histNewbie, in, HistoryQuery{Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(hist.Messages, "old-secret-166") {
		t.Fatalf("getHistory returned the hidden body: %v", prehistTexts(hist.Messages))
	}
	if !prehistHas(hist.Messages, "new-open-166") {
		t.Fatalf("getHistory dropped the post-join message: %v", prehistTexts(hist.Messages))
	}

	got, err := svc.GetMessages(ctx, histNewbie, in, []int32{oldID, freshID})
	if err != nil {
		t.Fatal(err)
	}
	if got.Messages[0].GetPredicateName() != mtproto.Predicate_messageEmpty {
		t.Fatalf("getMessages by id returned hidden body %q media=%v", got.Messages[0].GetMessage(), got.Messages[0].GetMedia() != nil)
	}
	if got.Messages[1].GetMessage() != "new-open-166" {
		t.Fatalf("post-join getMessages = %q", got.Messages[1].GetMessage())
	}
	if reply := got.Messages[1].GetReplyTo().GetReplyToMsgId(); reply != oldID && got.Messages[1].GetReplyTo().GetReplyToMsgId_INT32() != oldID {
		t.Fatalf("reply header should still name the old id, got %+v", got.Messages[1].GetReplyTo())
	}

	found, err := svc.SearchInChannel(ctx, histNewbie, ch.Id, ChannelSearch{Q: "old-secret-166", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(found.Messages, "old-secret-166") {
		t.Fatal("search returned the hidden body")
	}
	pinned, err := svc.SearchInChannel(ctx, histNewbie, ch.Id, ChannelSearch{Limit: 20, Filter: mtproto.MakeTLInputMessagesFilterPinned(nil).To_MessagesFilter()})
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(pinned.Messages, "old-secret-166") {
		t.Fatal("pinned search returned the hidden body")
	}
	ownerSearch, err := svc.SearchInChannel(ctx, histOwner, ch.Id, ChannelSearch{Q: "old-secret-166", Limit: 20})
	if err != nil || !prehistHas(ownerSearch.Messages, "old-secret-166") {
		t.Fatalf("owner lost the old message: %v", err)
	}

	diff, err := svc.Difference(ctx, histNewbie, in, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(diff.NewMessages, "old-secret-166") || updateHas(diff.OtherUpdates, "old-secret-166") {
		t.Fatalf("getChannelDifference returned the hidden body: msgs=%v updates=%d", prehistTexts(diff.NewMessages), len(diff.OtherUpdates))
	}
	if !prehistHas(diff.NewMessages, "new-open-166") && !updateHas(diff.OtherUpdates, "new-open-166") {
		t.Fatalf("getChannelDifference dropped the post-join message: %v", prehistTexts(diff.NewMessages))
	}

	if err = svc.ReadHistory(ctx, histOlder, in, oldID); err != nil {
		t.Fatal(err)
	}
	readers, err := svc.ReadParticipants(ctx, histOwner, ch.Id, oldID)
	if err != nil {
		t.Fatal(err)
	}
	if !readerHas(readers, histOlder) {
		t.Fatalf("member who was already in and read the message is missing: %+v", readers)
	}
	if readerHas(readers, histNewbie) {
		t.Fatalf("new member is on the old message read list before readHistory: %+v", readers)
	}
	if err = svc.ReadHistory(ctx, histNewbie, in, freshID); err != nil {
		t.Fatal(err)
	}
	readers, err = svc.ReadParticipants(ctx, histOwner, ch.Id, oldID)
	if err != nil {
		t.Fatal(err)
	}
	if readerHas(readers, histNewbie) {
		t.Fatalf("readHistory of a later message put the new member on the hidden message: %+v", readers)
	}
	freshReaders, err := svc.ReadParticipants(ctx, histOwner, ch.Id, freshID)
	if err != nil {
		t.Fatal(err)
	}
	if !readerHas(freshReaders, histNewbie) {
		t.Fatalf("new member did not count as a reader of the visible message: %+v", freshReaders)
	}

	if _, err = svc.TogglePreHistory(ctx, histOwner, 0, in, false); err != nil {
		t.Fatal(err)
	}
	opened, err := svc.GetMessages(ctx, histNewbie, in, []int32{oldID})
	if err != nil || opened.Messages[0].GetMessage() != "old-secret-166" {
		t.Fatalf("turning the setting off did not restore the body: %+v %v", opened, err)
	}
	if _, err = svc.TogglePreHistory(ctx, histOwner, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Leave(ctx, histNewbie, 0, in); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, histOwner, 0, in, []int64{histNewbie}, nil); err != nil {
		t.Fatal(err)
	}
	again, err := svc.GetMessages(ctx, histNewbie, in, []int32{oldID})
	if err != nil || again.Messages[0].GetPredicateName() != mtproto.Predicate_messageEmpty {
		t.Fatalf("rejoin exposed the hidden body: %+v %v", again, err)
	}
	readers, err = svc.ReadParticipants(ctx, histOwner, ch.Id, oldID)
	if err != nil {
		t.Fatal(err)
	}
	if readerHas(readers, histNewbie) {
		t.Fatalf("rejoin put the new member back on the hidden read list: %+v", readers)
	}
	dropChannelPts(t, svc, ch.Id, func(*PtsEvent) bool { return true })
	longNew, err := svc.Difference(ctx, histNewbie, in, 0, 100, false)
	if err != nil {
		t.Fatal(err)
	}
	if prehistHas(longNew.Messages, "old-secret-166") || prehistHas(longNew.NewMessages, "old-secret-166") {
		t.Fatalf("channelDifferenceTooLong returned the hidden body: %s %v", longNew.GetPredicateName(), prehistTexts(append(longNew.Messages, longNew.NewMessages...)))
	}

	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{ChangeInfo: true}).To_ChatAdminRights()
	if _, err = svc.EditAdmin(ctx, histOwner, 0, in, histNewbie, rights, ""); err != nil {
		t.Fatal(err)
	}
	asAdmin, err := svc.GetMessages(ctx, histNewbie, in, []int32{oldID})
	if err != nil || asAdmin.Messages[0].GetMessage() != "old-secret-166" {
		t.Fatalf("admin exemption failed: %+v %v", asAdmin, err)
	}

	news, err := svc.Create(ctx, CreateRequest{CreatorID: histOwner, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = svc.TogglePreHistory(ctx, histOwner, 0, inCh(news.Chats[0]), true); err == nil {
		t.Fatal("broadcast accepted hidden prehistory")
	}
}

func prehistHas(msgs []*mtproto.Message, text string) bool {
	for _, m := range msgs {
		if m.GetMessage() == text {
			return true
		}
	}
	return false
}

func updateHas(ups []*mtproto.Update, text string) bool {
	for _, u := range ups {
		if u.GetMessage_MESSAGE().GetMessage() == text {
			return true
		}
	}
	return false
}

func prehistTexts(msgs []*mtproto.Message) []string {
	out := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if m.GetMessage() != "" {
			out = append(out, m.GetMessage())
		}
	}
	return out
}

func readerHas(readers []Reader, id int64) bool {
	for _, r := range readers {
		if r.UserID == id {
			return true
		}
	}
	return false
}
