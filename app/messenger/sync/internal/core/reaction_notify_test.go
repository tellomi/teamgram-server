package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestReactionUpdateIsDeliveredToSessions(t *testing.T) {
	c := &SyncCore{}
	up := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
		MsgId_INT32: 3,
	}).To_Update())
	need, err := c.processUpdates(syncTypeUser, 9, false, up)
	if err != nil {
		t.Fatal(err)
	}
	if !need {
		t.Fatal("updateMessageReactions was not marked for session delivery")
	}
}

func TestDialogPinUpdateIsDeliveredToOtherSessions(t *testing.T) {
	c := &SyncCore{}
	channelID := int64(50)
	up := mtproto.MakeUpdatesByUpdatesChats(
		[]*mtproto.Chat{mtproto.MakeTLChannel(&mtproto.Chat{Id: channelID}).To_Chat()},
		mtproto.MakeTLUpdateDialogPinned(&mtproto.Update{
			Pinned: true,
			Peer_DIALOGPEER: mtproto.MakeTLDialogPeer(&mtproto.DialogPeer{
				Peer: mtproto.MakePeerChannel(channelID),
			}).To_DialogPeer(),
		}).To_Update(),
	)
	if len(up.GetChats()) != 1 || up.GetChats()[0].GetId() != channelID {
		t.Fatalf("chats %#v", up.GetChats())
	}
	need, err := c.processUpdates(syncTypeUserNotMe, 9, false, up)
	if err != nil {
		t.Fatal(err)
	}
	if !need {
		t.Fatal("updateDialogPinned was not marked for session delivery")
	}
}

func TestNotifySettingsUpdateIsDeliveredToOtherSessions(t *testing.T) {
	c := &SyncCore{}
	up := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNotifySettings(&mtproto.Update{
		NotifySettings: mtproto.MakeDefaultPeerNotifySettings(),
	}).To_Update())
	need, err := c.processUpdates(syncTypeUserNotMe, 9, false, up)
	if err != nil {
		t.Fatal(err)
	}
	if !need {
		t.Fatal("updateNotifySettings was not marked for session delivery")
	}
}
