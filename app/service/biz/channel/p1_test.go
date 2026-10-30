package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestUsernamesColorEmojiAndPaid(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Room")
	in := inCh(ch)

	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "roomname"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleUsername(ctx, 1, 0, in, "roomname", false); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetChannels(ctx, 1, []*mtproto.InputChannel{in})
	if err != nil || got.Chats[0].GetUsername().GetValue() != "" {
		t.Fatalf("username still public: %+v %v", got.GetChats(), err)
	}
	if _, err := svc.ToggleUsername(ctx, 1, 0, in, "missing", true); err == nil {
		t.Fatal("unknown username activated")
	}
	if _, err := svc.ToggleUsername(ctx, 1, 0, in, "roomname", true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReorderUsernames(ctx, 1, 0, in, []string{"roomname"}); err == nil {
		t.Fatal("identical order accepted")
	}
	if _, err := svc.DeactivateAllUsernames(ctx, 1, 0, in); err != nil {
		t.Fatal(err)
	}
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	names := full.Chats[0].GetUsernames()
	if len(names) != 1 || !names[0].GetActive() || names[0].GetUsername() != "roomname" {
		t.Fatalf("usernames %+v", names)
	}

	if _, err := svc.UpdateColor(ctx, 1, 0, &mtproto.TLChannelsUpdateColor{
		Channel:           in,
		Color_FLAGINT32:   wrapperspb.Int32(2),
		BackgroundEmojiId: wrapperspb.Int64(99),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateColor(ctx, 1, 0, &mtproto.TLChannelsUpdateColor{
		Channel:         in,
		Color_FLAGINT32: wrapperspb.Int32(20),
	}); err == nil {
		t.Fatal("color out of palette accepted")
	}
	if _, err := svc.UpdateEmojiStatus(ctx, 1, 0, in, mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{
		DocumentId: 42,
	}).To_EmojiStatus()); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdatePaidMessagesPrice(ctx, 1, 0, in, 15, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdatePaidMessagesPrice(ctx, 1, 0, in, -1, false); err == nil {
		t.Fatal("negative stars accepted")
	}
	full, err = svc.GetFullChannel(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	chat := full.Chats[0]
	if chat.GetColor_FLAGPEERCOLOR().GetColor().GetValue() != 2 || chat.GetEmojiStatus().GetDocumentId() != 42 {
		t.Fatalf("appearance color=%v emoji=%v", chat.GetColor_FLAGPEERCOLOR(), chat.GetEmojiStatus())
	}
	if !chat.GetBroadcastMessagesAllowed() || full.FullChat.GetSendPaidMessagesStars().GetValue() != 15 {
		t.Fatalf("paid chat=%v full=%v", chat.GetSendPaidMessagesStars(), full.FullChat.GetSendPaidMessagesStars())
	}
}

func TestAntispamBoostsAutotranslationForumView(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Group")
	in := inCh(ch)
	broadcast, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Feed", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleAntiSpam(ctx, 1, 0, inCh(broadcast.Chats[0]), true); err == nil {
		t.Fatal("broadcast antispam accepted")
	}
	if _, err := svc.ToggleAntiSpam(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReportAntiSpamFalsePositive(ctx, 1, in, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetBoostsToUnblockRestrictions(ctx, 1, 0, in, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleAutotranslation(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleViewForumAsMessages(ctx, 1, 0, in, true); err == nil {
		t.Fatal("non-forum view accepted")
	}
	forum, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Topics", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	fin := inCh(forum.Chats[0])
	if _, err := svc.ToggleViewForumAsMessages(ctx, 1, 0, fin, true); err != nil {
		t.Fatal(err)
	}
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	if !full.FullChat.GetAntispam() || full.FullChat.GetBoostsUnrestrict().GetValue() != 3 || !full.Chats[0].GetAutotranslation() {
		t.Fatalf("flags full=%v chat=%v", full.FullChat.GetAntispam(), full.Chats[0].GetAutotranslation())
	}
	ff, err := svc.GetFullChannel(ctx, 1, fin)
	if err != nil || !ff.FullChat.GetViewForumAsMessages() {
		t.Fatalf("forum view %+v %v", ff.GetFullChat().GetViewForumAsMessages(), err)
	}
	rows, err := svc.DialogsFor(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	var saw bool
	for _, row := range rows {
		if row.ChannelID == forum.Chats[0].GetId() && row.ViewForumAsMessages {
			saw = true
		}
	}
	if !saw {
		t.Fatal("dialog missing view_forum_as_messages")
	}
	if _, err := svc.SetMainProfileTab(ctx, 1, 0, in, mtproto.MakeTLProfileTabMedia(nil).To_ProfileTab()); err != nil {
		t.Fatal(err)
	}
	full, err = svc.GetFullChannel(ctx, 1, in)
	if err != nil || full.FullChat.GetMainTab().GetPredicateName() != mtproto.Predicate_profileTabMedia {
		t.Fatalf("main tab %v %v", full.GetFullChat().GetMainTab(), err)
	}
}

func TestRecommendationsLeftChannelsAndSpam(t *testing.T) {
	svc, ctx := newTest(t)
	home := megagroup(t, svc, ctx, "Home")
	in := inCh(home)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Send(ctx, 2, home.GetId(), 0, []Outgoing{{RandomID: 9, Message: &mtproto.Message{Message: "spam", Date: 1}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReportSpam(ctx, 1, in, &mtproto.InputPeer{
		PredicateName: mtproto.Predicate_inputPeerUser,
		UserId:        2,
	}, []int32{2}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ReportSpam(ctx, 2, in, &mtproto.InputPeer{
		PredicateName: mtproto.Predicate_inputPeerUser,
		UserId:        2,
	}, []int32{2}); err == nil {
		t.Fatal("member spam report accepted")
	}
	if _, err := svc.Leave(ctx, 2, 0, in); err != nil {
		t.Fatal(err)
	}
	left, err := svc.GetLeftChannels(ctx, 2, 0)
	if err != nil || len(left.Chats) != 1 || left.Chats[0].GetId() != home.GetId() {
		t.Fatalf("left %+v %v", left.GetChats(), err)
	}
	if _, err := svc.GetLeftChannels(ctx, 2, -1); err == nil {
		t.Fatal("negative offset accepted")
	}

	other, err := svc.Create(ctx, CreateRequest{CreatorID: 9, Title: "Public", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateUsername(ctx, 9, 0, inCh(other.Chats[0]), "publicfeed"); err != nil {
		t.Fatal(err)
	}
	rec, err := svc.GetChannelRecommendations(ctx, 1, nil)
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, chat := range rec.Chats {
		if chat.GetId() == other.Chats[0].GetId() {
			found = true
		}
		if chat.GetId() == home.GetId() {
			t.Fatal("recommendation included a joined channel")
		}
	}
	if !found {
		t.Fatalf("missing public recommendation: %+v", rec.Chats)
	}

	if _, err := svc.RecordPeerReport(ctx, 1, &mtproto.InputPeer{
		PredicateName: mtproto.Predicate_inputPeerUser,
		UserId:        4,
	}, "spam", "nope", 0); err != nil {
		t.Fatal(err)
	}
	if len(svc.PeerReports()) != 1 {
		t.Fatalf("reports %d", len(svc.PeerReports()))
	}
	if _, err := svc.RecordPeerReport(ctx, 1, nil, "", "", 0); err == nil {
		t.Fatal("empty peer accepted")
	}
}
