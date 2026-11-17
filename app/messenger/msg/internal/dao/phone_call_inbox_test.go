package dao

import (
	"strings"
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

func TestAnsweredCallCursorSQLDoesNotSkipEarlierUnread(t *testing.T) {
	sqlText := answeredCallCursorSQL("messages")
	if !strings.Contains(sqlText, "read_inbox_max_id < ?") {
		t.Fatal("cursor must move only forward onto this message")
	}
	if !strings.Contains(sqlText, "m.user_message_box_id > dialogs.read_inbox_max_id") || !strings.Contains(sqlText, "m.user_message_box_id < ?") {
		t.Fatal("an incoming message between the cursor and this service message must block the move")
	}
	if !strings.Contains(sqlText, "m.sender_user_id <> ?") {
		t.Fatal("the callee's own outgoing messages are not inbox unread")
	}
	if !strings.Contains(sqlText, "IF(top_message = ?, 0, unread_count)") {
		t.Fatal("unread_count must not be forced to 0 when a newer message is already the top")
	}
	if strings.Contains(sqlText, "unread_count = 0") {
		t.Fatal("absolute unread_count = 0 would mark every earlier message read")
	}
}
