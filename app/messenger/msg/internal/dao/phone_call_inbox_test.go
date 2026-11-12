package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestPhoneCallInboxUnreadDelta(t *testing.T) {
	hangup := mtproto.MakeTLPhoneCallDiscardReasonHangup(nil).To_PhoneCallDiscardReason()
	missed := mtproto.MakeTLPhoneCallDiscardReasonMissed(nil).To_PhoneCallDiscardReason()
	for _, video := range []bool{false, true} {
		answered := mtproto.MakeTLMessageService(&mtproto.Message{
			Action: mtproto.MakeMessageActionPhoneCall(video, 1, hangup, 5),
		}).To_Message()
		if got := phoneCallInboxUnread(answered); got != 0 {
			t.Fatalf("video=%v answered unread delta %d", video, got)
		}
		kept := mtproto.MakeTLMessageService(&mtproto.Message{
			Action: mtproto.MakeMessageActionPhoneCall(video, 1, missed, 0),
		}).To_Message()
		if got := phoneCallInboxUnread(kept); got != 1 {
			t.Fatalf("video=%v missed unread delta %d", video, got)
		}
		plain := mtproto.MakeTLMessage(&mtproto.Message{Message: "hi"}).To_Message()
		if got := phoneCallInboxUnread(plain); got != 1 {
			t.Fatalf("plain unread delta %d", got)
		}
	}
}
