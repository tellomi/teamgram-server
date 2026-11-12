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
