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

func TestQuickReplyUpdatesAreDeliveredToOtherSessions(t *testing.T) {
	c := &SyncCore{}
	cases := []*mtproto.Update{
		mtproto.MakeTLUpdateNewQuickReply(&mtproto.Update{
			QuickReply: mtproto.MakeTLQuickReply(&mtproto.QuickReply{ShortcutId: 1, Shortcut: "hi"}).To_QuickReply(),
		}).To_Update(),
		mtproto.MakeTLUpdateQuickReplyMessage(&mtproto.Update{
			Message_MESSAGE: mtproto.MakeTLMessage(&mtproto.Message{Id: 2, Message: "hi"}).To_Message(),
		}).To_Update(),
		mtproto.MakeTLUpdateQuickReplies(&mtproto.Update{
			QuickReplies: []*mtproto.QuickReply{mtproto.MakeTLQuickReply(&mtproto.QuickReply{ShortcutId: 1}).To_QuickReply()},
		}).To_Update(),
		mtproto.MakeTLUpdateDeleteQuickReply(&mtproto.Update{ShortcutId: 1}).To_Update(),
		mtproto.MakeTLUpdateDeleteQuickReplyMessages(&mtproto.Update{ShortcutId: 1, Messages: []int32{2}}).To_Update(),
	}
	for _, up := range cases {
		need, err := c.processUpdates(syncTypeUserNotMe, 9, false, mtproto.MakeUpdatesByUpdates(up))
		if err != nil || !need {
			t.Fatalf("%s delivered %v %v", up.GetPredicateName(), need, err)
		}
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
