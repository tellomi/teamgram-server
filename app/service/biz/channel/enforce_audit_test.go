package channel

import (
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
)

func TestMegagroupSendRightsPinAndTopics(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Rights")
	in := inCh(ch)
	_, _ = svc.Invite(ctx, 1, 0, in, []int64{2}, nil)

	photos := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendPhotos: true}).To_ChatBannedRights()
	if _, err := svc.EditDefaultBanned(ctx, 1, 0, in, photos); err != nil {
		t.Fatal(err)
	}
	pic := &mtproto.Message{Message: "cap", Date: int32(time.Now().Unix()), Media: mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia()}
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 1, Message: pic}}); err == nil || !strings.Contains(err.Error(), "CHAT_SEND_PHOTOS_FORBIDDEN") {
		t.Fatalf("default photo ban: %v", err)
	}
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 2, Message: &mtproto.Message{Message: "text", Date: int32(time.Now().Unix())}}}); err != nil {
		t.Fatalf("text blocked by a photo ban: %v", err)
	}
	if _, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 3, Message: pic}}); err != nil {
		t.Fatalf("creator photo: %v", err)
	}

	until := int32(time.Now().Unix()) + 3600
	userBan := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{SendVideos: true, UntilDate: until}).To_ChatBannedRights()
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, userBan); err != nil {
		t.Fatal(err)
	}
	vid := &mtproto.Message{Date: int32(time.Now().Unix()), Media: mtproto.MakeTLMessageMediaDocument(&mtproto.MessageMedia{
		Document: mtproto.MakeTLDocument(&mtproto.Document{MimeType: "video/mp4", Attributes: []*mtproto.DocumentAttribute{
			mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{W: 10, H: 10}).To_DocumentAttribute(),
		}}).To_Document(),
	}).To_MessageMedia()}
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 4, Message: vid}}); err == nil || !strings.Contains(err.Error(), "CHAT_SEND_VIDEOS_FORBIDDEN") || strings.Contains(err.Error(), "CHAT_SEND_VIDEOS_FORBIDDEN_") {
		t.Fatalf("per-user video until stays the plain code: %v", err)
	}

	open := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{}).To_ChatBannedRights()
	if _, err := svc.EditDefaultBanned(ctx, 1, 0, in, open); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditBanned(ctx, 1, 0, in, 2, open); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 5, Message: &mtproto.Message{Message: "pin me", Date: int32(time.Now().Unix())}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	if _, err = svc.PinMessage(ctx, 2, 0, ch.Id, msgID, false); err != nil {
		t.Fatalf("member pin when allowed: %v", err)
	}
	locked := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{PinMessages: true, ManageTopics: true}).To_ChatBannedRights()
	if _, err = svc.EditDefaultBanned(ctx, 1, 0, in, locked); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.PinMessage(ctx, 2, 0, ch.Id, msgID, true); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("banned pin: %v", err)
	}

	forum, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Forum", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	fch := forum.Chats[0]
	fin := inCh(fch)
	_, _ = svc.Invite(ctx, 1, 0, fin, []int64{2}, nil)
	if _, err = svc.EditDefaultBanned(ctx, 1, 0, fin, locked); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.CreateTopic(ctx, 2, 0, fch.Id, "Nope", 1, 0); err != mtproto.ErrChatAdminRequired {
		t.Fatalf("banned topic: %v", err)
	}
	if _, _, err = svc.CreateTopic(ctx, 1, 0, fch.Id, "Yes", 1, 0); err != nil {
		t.Fatalf("creator topic: %v", err)
	}
}

func TestInviteStartDate(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Later")
	in := inCh(ch)
	exported, err := svc.ExportInvite(ctx, 1, in, ExportRequest{Title: "later", TitleSet: true})
	if err != nil {
		t.Fatal(err)
	}
	hash := strings.TrimPrefix(exported.GetLink(), brand.LinkBase()+"/+")
	inv, err := svc.store.GetInviteByLink(ctx, hash)
	if err != nil || inv == nil {
		t.Fatal(err)
	}
	inv.StartDate = time.Now().Unix() + 3600
	if err = svc.store.UpdateInvite(ctx, inv); err != nil {
		t.Fatal(err)
	}
	if _, _, err = svc.CheckInvite(ctx, 3, hash); err == nil || !strings.Contains(err.Error(), "INVITE_HASH_EXPIRED") {
		t.Fatalf("check before start: %v", err)
	}
	if _, err = svc.ImportInvite(ctx, 3, 0, hash); err == nil || !strings.Contains(err.Error(), "INVITE_HASH_EXPIRED") {
		t.Fatalf("import before start: %v", err)
	}
	inv.StartDate = time.Now().Unix() - 10
	if err = svc.store.UpdateInvite(ctx, inv); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ImportInvite(ctx, 3, 0, hash); err != nil {
		t.Fatalf("import after start: %v", err)
	}
}

func TestHiddenPrehistoryOnReadPaths(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "History")
	in := inCh(ch)
	oldDate := int32(time.Now().Unix()) - 100
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "secret old", Date: oldDate, Media: mtproto.MakeTLMessageMediaPhoto(&mtproto.MessageMedia{}).To_MessageMedia()}}})
	if err != nil {
		t.Fatal(err)
	}
	oldID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	reply, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 2, Message: &mtproto.Message{
		Message: "old reply", Date: oldDate,
		ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{ReplyToMsgId: oldID}).To_MessageReplyHeader(),
	}}})
	if err != nil {
		t.Fatal(err)
	}
	replyID := reply.Updates[0].GetMessage_MESSAGE().GetId()
	_, _ = svc.Invite(ctx, 1, 0, in, []int64{2}, nil)
	if _, err = svc.TogglePreHistory(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	fresh, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 3, Message: &mtproto.Message{Message: "after join", Date: int32(time.Now().Unix()) + 5}}})
	if err != nil {
		t.Fatal(err)
	}
	freshID := fresh.Updates[0].GetMessage_MESSAGE().GetId()

	found, err := svc.SearchInChannel(ctx, 2, ch.Id, ChannelSearch{Q: "secret", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if containsMsg(found.Messages, oldID) {
		t.Fatal("search returned prehistory")
	}
	media, err := svc.SearchInChannel(ctx, 2, ch.Id, ChannelSearch{Limit: 20, Filter: mtproto.MakeTLInputMessagesFilterPhotos(nil).To_MessagesFilter()})
	if err != nil {
		t.Fatal(err)
	}
	if containsMsg(media.Messages, oldID) {
		t.Fatal("media search returned prehistory")
	}
	adminSearch, _ := svc.SearchInChannel(ctx, 1, ch.Id, ChannelSearch{Q: "secret", Limit: 20})
	if !containsMsg(adminSearch.Messages, oldID) {
		t.Fatal("creator lost prehistory")
	}
	got, err := svc.GetMessages(ctx, 2, in, []int32{oldID, freshID})
	if err != nil || got.Messages[0].GetPredicateName() != mtproto.Predicate_messageEmpty || got.Messages[1].GetId() != freshID {
		t.Fatalf("getMessages %+v %v", got, err)
	}
	replies, err := svc.Replies(ctx, 2, ch.Id, oldID, HistoryQuery{Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	if containsMsg(replies.Messages, replyID) || containsMsg(replies.Messages, oldID) {
		t.Fatalf("replies leaked prehistory %+v", replies.Messages)
	}
	if _, err = svc.DiscussionMessage(ctx, 2, ch.Id, oldID); err != mtproto.ErrMessageIdInvalid {
		t.Fatalf("discussion: %v", err)
	}
	if _, err = svc.DiscussionMessage(ctx, 1, ch.Id, oldID); err != nil {
		t.Fatalf("creator discussion: %v", err)
	}
	gate := svc.PrehistoryGate(ctx, 2, ch.Id)
	if !gate.Blocks(oldDate) || gate.Blocks(int32(time.Now().Unix())+5) {
		t.Fatalf("gate %+v", gate)
	}
	if svc.PrehistoryGate(ctx, 1, ch.Id).Blocks(oldDate) {
		t.Fatal("creator gate blocked")
	}
}

func containsMsg(msgs []*mtproto.Message, id int32) bool {
	for _, m := range msgs {
		if m.GetId() == id {
			return true
		}
	}
	return false
}
