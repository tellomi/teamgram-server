package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestUpdateNotifySettingsStoresForumTopicAndPushesOtherSessions(t *testing.T) {
	prev := channel.Shared()
	chSvc := channel.New(channel.NewMemoryStore())
	channel.Use(chSvc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	created, err := chSvc.Create(ctx, channel.CreateRequest{CreatorID: 7, Title: "Forum", Megagroup: true, Forum: true})
	if err != nil {
		t.Fatal(err)
	}
	channelID := created.Chats[0].GetId()

	users := &notifyUserFake{stored: mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		MuteUntil: wrapperspb.Int32(2147483647),
	}).To_PeerNotifySettings()}
	pushes := &notifySyncFake{}
	c := notifyCore(users, pushes, "android")
	ok, err := c.AccountUpdateNotifySettings(&mtproto.TLAccountUpdateNotifySettings{
		Peer: mtproto.MakeTLInputNotifyForumTopic(&mtproto.InputNotifyPeer{
			Peer:     mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: channelID}).To_InputPeer(),
			TopMsgId: 15,
		}).To_InputNotifyPeer(),
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			MuteUntil: wrapperspb.Int32(2147483647),
		}).To_InputPeerNotifySettings(),
	})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("update: %v %v", ok, err)
	}
	if users.setReq == nil || users.setReq.GetTopMsgId() != 15 || users.setReq.GetPeerType() != mtproto.PEER_CHANNEL || users.setReq.GetPeerId() != channelID {
		t.Fatalf("stored request %+v", users.setReq)
	}
	if users.getReq == nil || users.getReq.GetTopMsgId() != 15 {
		t.Fatalf("read-back %+v", users.getReq)
	}
	if len(pushes.got) != 1 {
		t.Fatalf("pushes %d", len(pushes.got))
	}
	var saw bool
	for _, upd := range pushes.got[0].GetUpdates().GetUpdates() {
		if upd.GetPredicateName() != mtproto.Predicate_updateNotifySettings {
			continue
		}
		saw = true
		np := upd.GetPeer_NOTIFYPEER()
		if np.GetPredicateName() != mtproto.Predicate_notifyForumTopic || np.GetTopMsgId() != 15 || np.GetPeer().GetChannelId() != channelID {
			t.Fatalf("pushed peer %+v", np)
		}
		if upd.GetNotifySettings().GetMuteUntil().GetValue() != 2147483647 {
			t.Fatalf("pushed settings %+v", upd.GetNotifySettings())
		}
	}
	if !saw {
		t.Fatal("other sessions were not sent updateNotifySettings")
	}
	if len(pushes.got[0].GetUpdates().GetChats()) == 0 {
		t.Fatal("the channel card was not attached for the other session")
	}
}

func TestUpdateNotifySettingsRejectsForumTopicWithoutAnId(t *testing.T) {
	c := notifyCore(&notifyUserFake{stored: mtproto.MakeDefaultPeerNotifySettings()}, &notifySyncFake{}, "ios")
	_, err := c.AccountUpdateNotifySettings(&mtproto.TLAccountUpdateNotifySettings{
		Peer: mtproto.MakeTLInputNotifyForumTopic(&mtproto.InputNotifyPeer{
			Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 4}).To_InputPeer(),
		}).To_InputNotifyPeer(),
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			Silent: mtproto.BoolTrue,
		}).To_InputPeerNotifySettings(),
	})
	if err != mtproto.ErrTopicIdInvalid {
		t.Fatalf("err %v", err)
	}
}

func TestGetNotifySettingsReadsTheForumTopic(t *testing.T) {
	users := &notifyUserFake{stored: mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		MuteUntil: wrapperspb.Int32(20),
	}).To_PeerNotifySettings()}
	c := notifyCore(users, &notifySyncFake{}, "android")
	got, err := c.AccountGetNotifySettings(&mtproto.TLAccountGetNotifySettings{
		Peer: mtproto.MakeTLInputNotifyForumTopic(&mtproto.InputNotifyPeer{
			Peer:     mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 4}).To_InputPeer(),
			TopMsgId: 8,
		}).To_InputNotifyPeer(),
	})
	if err != nil || got.GetMuteUntil().GetValue() != 20 {
		t.Fatalf("get %+v %v", got, err)
	}
	if users.getReq == nil || users.getReq.GetTopMsgId() != 8 || users.getReq.GetPeerId() != 4 {
		t.Fatalf("request %+v", users.getReq)
	}

	_, err = c.AccountGetNotifySettings(&mtproto.TLAccountGetNotifySettings{
		Peer: mtproto.MakeTLInputNotifyForumTopic(&mtproto.InputNotifyPeer{TopMsgId: 8}).To_InputNotifyPeer(),
	})
	if err != mtproto.ErrPeerIdInvalid {
		t.Fatalf("nil peer %v", err)
	}
}

func TestGetNotifyExceptionsEmitsForumTopics(t *testing.T) {
	users := &notifyUserFake{all: &user.Vector_PeerPeerNotifySettings{
		Datas: []*user.PeerPeerNotifySettings{
			user.MakeTLPeerPeerNotifySettings(&user.PeerPeerNotifySettings{
				PeerType: mtproto.PEER_CHANNEL,
				PeerId:   4,
				Settings: mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
					MuteUntil: wrapperspb.Int32(0),
				}).To_PeerNotifySettings(),
			}).To_PeerPeerNotifySettings(),
			user.MakeTLPeerPeerNotifySettings(&user.PeerPeerNotifySettings{
				PeerType: mtproto.PEER_CHANNEL,
				PeerId:   4,
				TopMsgId: 8,
				Settings: mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
					MuteUntil: wrapperspb.Int32(2147483647),
				}).To_PeerNotifySettings(),
			}).To_PeerPeerNotifySettings(),
		},
	}}
	c := notifyCore(users, &notifySyncFake{}, "android")
	up, err := c.AccountGetNotifyExceptions(&mtproto.TLAccountGetNotifyExceptions{})
	if err != nil {
		t.Fatal(err)
	}
	var dialog, topic bool
	for _, upd := range up.GetUpdates() {
		np := upd.GetPeer_NOTIFYPEER()
		switch np.GetPredicateName() {
		case mtproto.Predicate_notifyPeer:
			dialog = np.GetPeer().GetChannelId() == 4 && upd.GetNotifySettings().GetMuteUntil().GetValue() == 0
		case mtproto.Predicate_notifyForumTopic:
			topic = np.GetTopMsgId() == 8 && upd.GetNotifySettings().GetMuteUntil().GetValue() == 2147483647
		}
	}
	if !dialog || !topic {
		t.Fatalf("dialog %v topic %v updates %+v", dialog, topic, up.GetUpdates())
	}
}
