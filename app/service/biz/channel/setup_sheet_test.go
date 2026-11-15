package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMigrateKeepsAdminAndPhoto(t *testing.T) {
	svc, ctx := newTest(t)
	rights := int32(mtproto.MakeChatAdminRightsHelper(mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		ChangeInfo: true,
		BanUsers:   true,
	}).To_ChatAdminRights()))
	migrated, err := svc.Migrate(ctx, MigrateRequest{
		Actor:     1,
		ChatID:    77,
		Title:     "Old room",
		PhotoID:   4242,
		CreatorID: 1,
		Members: []LegacyMember{
			{UserID: 2, Admin: true, AdminRights: rights},
			{UserID: 3},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	in := input(migrated.ChannelID, migrated.AccessHash)
	got, err := svc.GetParticipant(ctx, 1, in, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer())
	if err != nil {
		t.Fatal(err)
	}
	if got.GetParticipant().GetPredicateName() != mtproto.Predicate_channelParticipantAdmin {
		t.Fatalf("admin became %s", got.GetParticipant().GetPredicateName())
	}
	if !got.GetParticipant().GetAdminRights().GetChangeInfo() || !got.GetParticipant().GetAdminRights().GetBanUsers() {
		t.Fatalf("rights %+v", got.GetParticipant().GetAdminRights())
	}
	member, err := svc.GetParticipant(ctx, 1, in, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 3}).To_InputPeer())
	if err != nil || member.GetParticipant().GetPredicateName() != mtproto.Predicate_channelParticipant {
		t.Fatalf("member %+v %v", member, err)
	}
	full, err := svc.GetFullChannel(ctx, 1, in)
	if err != nil {
		t.Fatal(err)
	}
	if full.FullChat.GetChatPhoto().GetId() != 4242 {
		t.Fatalf("full photo %d", full.FullChat.GetChatPhoto().GetId())
	}
	if full.Chats[0].GetPhoto().GetPhotoId() != 4242 {
		t.Fatalf("chat photo %+v", full.Chats[0].GetPhoto())
	}
}

func TestCreateChannelKeepsGeoPoint(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{
		CreatorID: 1,
		Title:     "Pin",
		Megagroup: true,
		Address:   "Chengdu",
		Lat:       30.57,
		Long:      104.06,
		HasPoint:  true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	if !ch.GetHasGeo() {
		t.Fatal("has_geo")
	}
	full, err := svc.GetFullChannel(ctx, 1, inCh(ch))
	if err != nil {
		t.Fatal(err)
	}
	pt := full.FullChat.GetLocation().GetGeoPoint()
	if pt.GetLat() != 30.57 || pt.GetLong() != 104.06 || full.FullChat.GetLocation().GetAddress() != "Chengdu" {
		t.Fatalf("location %+v", full.FullChat.GetLocation())
	}
}

func TestToggleForumKeepsSystemGeneralAndUserNamedGeneral(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Topics", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	hidden := true
	if _, err = svc.EditTopic(ctx, 1, ch.Id, 1, "", nil, nil, &hidden); err != nil {
		t.Fatal(err)
	}
	hidden = false
	if _, err = svc.EditTopic(ctx, 1, ch.Id, 1, "", nil, nil, &hidden); err != nil {
		t.Fatal(err)
	}
	_, user, err := svc.CreateTopic(ctx, 1, 0, ch.Id, "General", 0x6FB9F0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if user == nil || user.ID == generalTopicID || user.System {
		t.Fatalf("user topic %#v", user)
	}
	if _, err = svc.ToggleForum(ctx, 1, in, false); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.ToggleForum(ctx, 1, in, true); err != nil {
		t.Fatal(err)
	}
	topics, _, err := svc.ListTopics(ctx, 1, ch.Id, "", 20, 0, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	var system, named int
	for _, topic := range topics {
		if topic.GetId() == generalTopicID {
			system++
			if topic.GetHidden() {
				t.Fatal("system general hidden state was replaced")
			}
		}
		if topic.GetId() == user.ID && topic.GetTitle() == "General" {
			named++
		}
	}
	if system != 1 || named != 1 {
		t.Fatalf("system %d user %d %#v", system, named, topics)
	}
}
