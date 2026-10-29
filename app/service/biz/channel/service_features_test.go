package channel

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func megagroup(t *testing.T, svc *Service, ctx context.Context, title string) *mtproto.Chat {
	t.Helper()
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: title, Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	return up.Chats[0]
}

func inCh(ch *mtproto.Chat) *mtproto.InputChannel {
	return input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
}

func TestAdminLogStickersSlowModeAndHistory(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Ops")
	in := inCh(ch)
	if _, err := svc.EditTitle(ctx, 1, 0, in, "Ops2"); err != nil {
		t.Fatal(err)
	}
	log, err := svc.GetAdminLog(ctx, 1, in, "", nil, nil, 0, 0, 20)
	if err != nil || len(log.Events) == 0 {
		t.Fatalf("admin log %#v %v", log, err)
	}
	if _, err := svc.SetStickers(ctx, 1, in, &mtproto.InputStickerSet{PredicateName: mtproto.Predicate_inputStickerSetID, Id: 77}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleSlowMode(ctx, 1, 0, in, 10); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleSlowMode(ctx, 1, 0, in, 7); err == nil {
		t.Fatal("invalid slowmode accepted")
	}
	_, _ = svc.Invite(ctx, 1, 0, in, []int64{2}, nil)
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "a", Date: 1}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, 2, ch.Id, 0, []Outgoing{{RandomID: 2, Message: &mtproto.Message{Message: "b", Date: 1}}}); err == nil {
		t.Fatal("slowmode did not wait")
	}
	if err := svc.ReadContents(ctx, 2, in, []int32{1}); err != nil {
		t.Fatal(err)
	}
	pts, count, err := svc.DeleteHistory(ctx, 2, 0, in, 2, false)
	if err != nil || count != 0 || pts == 0 {
		t.Fatalf("self history pts=%d count=%d err=%v", pts, count, err)
	}
	pts, count, err = svc.DeleteHistory(ctx, 1, 0, in, 0, true)
	if err != nil || count == 0 {
		t.Fatalf("everyone history pts=%d count=%d err=%v", pts, count, err)
	}
}

func TestJoinRequestCreatorDiscussionLocationStats(t *testing.T) {
	svc, ctx := newTest(t)
	broadcast, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	b := broadcast.Chats[0]
	if _, err := svc.UpdateUsername(ctx, 1, 0, inCh(b), "newsroom"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleJoinRequest(ctx, 1, 0, inCh(b), true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleJoinToSend(ctx, 1, 0, inCh(b), true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Join(ctx, 4, 0, inCh(b)); err != nil {
		t.Fatal(err)
	}
	parts, _ := svc.GetParticipants(ctx, 1, inCh(b), nil, 0, 50, 0)
	if parts.Count != 1 {
		t.Fatalf("join request should not add a member, count=%d", parts.Count)
	}
	if err := svc.ApproveJoin(ctx, 1, 0, inCh(b), 4, true); err != nil {
		t.Fatal(err)
	}
	parts, _ = svc.GetParticipants(ctx, 1, inCh(b), nil, 0, 50, 0)
	if parts.Count != 2 {
		t.Fatalf("approved member missing, count=%d", parts.Count)
	}
	if err := svc.EditLocation(ctx, 1, inCh(b), 1.5, 2.5, "Hall"); err != nil {
		t.Fatal(err)
	}
	full, err := svc.GetFullChannel(ctx, 1, inCh(b))
	if err != nil || full.FullChat.GetLocation().GetAddress() != "Hall" {
		t.Fatalf("location %#v %v", full, err)
	}
	group := megagroup(t, svc, ctx, "Talk")
	if err := svc.SetDiscussionGroup(ctx, 1, inCh(b), inCh(group)); err != nil {
		t.Fatal(err)
	}
	full, _ = svc.GetFullChannel(ctx, 1, inCh(b))
	if full.FullChat.GetLinkedChatId().GetValue() != group.Id {
		t.Fatalf("linked %d", full.FullChat.GetLinkedChatId().GetValue())
	}
	groups, err := svc.GroupsForDiscussion(ctx, 1)
	if err != nil || len(groups.Chats) == 0 {
		t.Fatalf("discussion groups %#v %v", groups, err)
	}
	stats, err := svc.BroadcastStats(ctx, 1, inCh(b))
	if err != nil || stats.Followers.Current < 1 || stats.GrowthGraph == nil {
		t.Fatalf("stats %#v %v", stats, err)
	}
	buf := mtproto.NewEncodeBuf(2048)
	if err := stats.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleParticipantsHidden(ctx, 1, 0, inCh(b), true); err != nil {
		t.Fatal(err)
	}
	hidden, err := svc.GetParticipants(ctx, 4, inCh(b), nil, 0, 50, 0)
	if err != nil || len(hidden.Participants) != 0 {
		t.Fatalf("hidden participants %#v %v", hidden, err)
	}
	inactive, err := svc.InactiveChannels(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	_ = inactive
}

func TestEditCreatorFutureGigagroupPrehistory(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Owners")
	_, _ = svc.Invite(ctx, 1, 0, inCh(ch), []int64{2}, nil)
	next, err := svc.FutureCreator(ctx, 1, inCh(ch))
	if err != nil || next != 2 {
		t.Fatalf("future creator %d %v", next, err)
	}
	if _, err := svc.EditCreator(ctx, 1, 0, inCh(ch), 2, nil); err == nil {
		t.Fatal("missing password")
	}
	pw := &mtproto.InputCheckPasswordSRP{A: []byte{1, 2, 3}}
	if _, err := svc.EditCreator(ctx, 1, 0, inCh(ch), 2, pw); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetParticipant(ctx, 2, inCh(ch), mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer())
	if err != nil || got.Participant.GetPredicateName() != mtproto.Predicate_channelParticipantCreator {
		t.Fatalf("creator transfer %#v %v", got, err)
	}
	if _, err := svc.ConvertToGigagroup(ctx, 2, 0, inCh(ch)); err != nil {
		t.Fatal(err)
	}
	view := svc.ChatFor(ctx, 2, mustChannel(t, svc, ctx, ch.Id))
	if !view.Gigagroup {
		t.Fatal("gigagroup flag missing")
	}
	if _, err := svc.TogglePreHistory(ctx, 2, 0, inCh(ch), true); err != nil {
		t.Fatal(err)
	}
	full, _ := svc.GetFullChannel(ctx, 2, inCh(ch))
	if !full.FullChat.HiddenPrehistory {
		t.Fatal("prehistory not hidden")
	}
}

func TestTopicsReactionsPinsSearchSendAsPersonal(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Forum", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	_, topic, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "Hello", 0x6FB9F0, 0)
	if err != nil || topic.ID == 0 {
		t.Fatalf("topic %#v %v", topic, err)
	}
	topics, _, err := svc.ListTopics(ctx, 1, ch.Id, "hel", 10)
	if err != nil || len(topics) != 1 || topics[0].Title != "Hello" {
		t.Fatalf("topics %#v %v", topics, err)
	}
	if _, err := svc.EditTopic(ctx, 1, ch.Id, topic.ID, "Hello!", nil); err != nil {
		t.Fatal(err)
	}
	if err := svc.PinTopic(ctx, 1, ch.Id, topic.ID, true); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 5, Message: &mtproto.Message{Message: "post #tag", Date: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].Message_MESSAGE.Id
	if _, err := svc.PinMessage(ctx, 1, 0, ch.Id, msgID, false); err != nil {
		t.Fatal(err)
	}
	full, _ := svc.GetFullChannel(ctx, 1, inCh(ch))
	if full.FullChat.GetPinnedMsgId().GetValue() != msgID {
		t.Fatalf("pin %d", full.FullChat.GetPinnedMsgId().GetValue())
	}
	react, err := svc.React(ctx, 1, ch.Id, msgID, []string{"👍"}, false)
	if err != nil || len(react.Updates) == 0 {
		t.Fatalf("react %#v %v", react, err)
	}
	again, err := svc.ReactionsFor(ctx, 1, ch.Id, []int32{msgID})
	if err != nil || again.Updates[0].GetReactions_MESSAGEREACTIONS().GetResults()[0].Count != 1 {
		t.Fatalf("reactions %#v %v", again, err)
	}
	reply := &mtproto.Message{
		Message: "reply",
		Date:    4,
		ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{ReplyToMsgId: msgID, ReplyToMsgId_INT32: msgID}).To_MessageReplyHeader(),
	}
	if _, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 6, Message: reply}}); err != nil {
		t.Fatal(err)
	}
	replies, err := svc.Replies(ctx, 1, ch.Id, msgID, 0, 10)
	if err != nil || replies.Count != 1 {
		t.Fatalf("replies %#v %v", replies, err)
	}
	disc, err := svc.DiscussionMessage(ctx, 1, ch.Id, msgID)
	if err != nil || len(disc.Messages) < 2 {
		t.Fatalf("discussion %#v %v", disc, err)
	}
	found, err := svc.SearchChannel(ctx, 1, ch.Id, "tag", 0, 0, 10)
	if err != nil || found.Count == 0 {
		t.Fatalf("search %#v %v", found, err)
	}
	if _, err := svc.UpdateUsername(ctx, 1, 0, inCh(ch), "forumtag"); err != nil {
		t.Fatal(err)
	}
	posts, err := svc.SearchPosts(ctx, 1, "", "tag", 0, 10)
	if err != nil || posts.Count == 0 {
		t.Fatalf("search posts %#v %v", posts, err)
	}
	chats, ids, err := svc.SendAs(ctx, 1)
	if err != nil || len(ids) < 2 || len(chats) == 0 {
		t.Fatalf("send as %#v %v %v", chats, ids, err)
	}
	news, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Personal", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateUsername(ctx, 1, 0, inCh(news.Chats[0]), "personalfeed"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, 1, news.Chats[0].Id, 0, []Outgoing{{RandomID: 8, Message: &mtproto.Message{Message: "bio", Date: 5}}}); err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPersonalChannel(ctx, 1, inCh(news.Chats[0])); err != nil {
		t.Fatal(err)
	}
	hist, err := svc.PersonalHistory(ctx, 1, 1, 10, 0, 0)
	if err != nil || hist.Count == 0 {
		t.Fatalf("personal history %#v %v", hist, err)
	}
	pts, count, offset, err := svc.DeleteParticipantHistory(ctx, 1, 0, inCh(ch), 1)
	if err != nil || count == 0 || offset != 0 || pts == 0 {
		t.Fatalf("participant history pts=%d count=%d err=%v", pts, count, err)
	}
	mega, err := svc.MegagroupStats(ctx, 1, inCh(ch))
	if err != nil || mega.Members.Current < 1 {
		t.Fatalf("megagroup stats %#v %v", mega, err)
	}
}

func TestResolveUploadedAndExistingPhoto(t *testing.T) {
	ctx := context.Background()
	uploaded := &mtproto.Photo{Id: 55, PredicateName: mtproto.Predicate_photo}
	id, photo, clear, err := ResolveChatPhoto(ctx, 1, &mtproto.InputChatPhoto{
		PredicateName: mtproto.Predicate_inputChatUploadedPhoto,
		File:          &mtproto.InputFile{Id_INT64: 9, Name: "a.jpg"},
	}, func(file, video *mtproto.InputFile, start float64) (*mtproto.Photo, error) {
		if file.GetId_INT64() != 9 {
			t.Fatalf("file id %d", file.GetId_INT64())
		}
		return uploaded, nil
	}, nil)
	if err != nil || clear || id != 55 || photo.GetId() != 55 {
		t.Fatalf("upload id=%d clear=%v err=%v", id, clear, err)
	}
	id, _, clear, err = ResolveChatPhoto(ctx, 1, &mtproto.InputChatPhoto{
		PredicateName: mtproto.Predicate_inputChatPhoto,
		Id:            &mtproto.InputPhoto{PredicateName: mtproto.Predicate_inputPhoto, Id: 55},
	}, nil, func(photoID int64) (*mtproto.Photo, error) {
		return &mtproto.Photo{Id: photoID}, nil
	})
	if err != nil || clear || id != 55 {
		t.Fatalf("existing id=%d err=%v", id, err)
	}
	_, _, clear, err = ResolveChatPhoto(ctx, 1, &mtproto.InputChatPhoto{PredicateName: mtproto.Predicate_inputChatPhotoEmpty}, nil, nil)
	if err != nil || !clear {
		t.Fatal(err)
	}
}

func mustChannel(t *testing.T, svc *Service, ctx context.Context, id int64) *Channel {
	t.Helper()
	ch, err := svc.reload(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	return ch
}
