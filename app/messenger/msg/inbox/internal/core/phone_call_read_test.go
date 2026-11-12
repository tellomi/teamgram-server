package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAnsweredCallReadUpdateVoiceAndVideo(t *testing.T) {
	for _, video := range []bool{false, true} {
		action := mtproto.MakeMessageActionPhoneCall(video, 7, mtproto.MakeTLPhoneCallDiscardReasonHangup(nil).To_PhoneCallDiscardReason(), 8)
		if !mtproto.AnsweredPhoneCall(action) {
			t.Fatalf("video=%v should be read", video)
		}
		upd := answeredCallReadUpdate(11, 44, 90)
		if upd.GetPredicateName() != mtproto.Predicate_updateReadHistoryInbox {
			t.Fatalf("update %s", upd.GetPredicateName())
		}
		if upd.GetPeer_PEER().GetUserId() != 11 || upd.GetMaxId() != 44 || upd.GetStillUnreadCount() != 0 || upd.GetPts_INT32() != 90 || upd.GetPtsCount() != 1 {
			t.Fatalf("update %+v", upd)
		}
	}
}

func TestMissedAndBusyStayUnread(t *testing.T) {
	for _, video := range []bool{false, true} {
		missed := mtproto.MakeMessageActionPhoneCall(video, 7, mtproto.MakeTLPhoneCallDiscardReasonMissed(nil).To_PhoneCallDiscardReason(), 0)
		busy := mtproto.MakeMessageActionPhoneCall(video, 7, mtproto.MakeTLPhoneCallDiscardReasonBusy(nil).To_PhoneCallDiscardReason(), 0)
		declined := mtproto.MakeMessageActionPhoneCall(video, 7, mtproto.MakeTLPhoneCallDiscardReasonHangup(nil).To_PhoneCallDiscardReason(), 0)
		if mtproto.AnsweredPhoneCall(missed) || mtproto.AnsweredPhoneCall(busy) || mtproto.AnsweredPhoneCall(declined) {
			t.Fatalf("video=%v missed/busy/declined must stay unread", video)
		}
	}
}
