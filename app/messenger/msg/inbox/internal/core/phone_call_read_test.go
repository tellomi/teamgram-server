package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestAnsweredCallReadFollowsNewMessage(t *testing.T) {
	for _, video := range []bool{false, true} {
		action := mtproto.MakeMessageActionPhoneCall(video, 7, mtproto.MakeTLPhoneCallDiscardReasonHangup(nil).To_PhoneCallDiscardReason(), 8)
		if !mtproto.AnsweredPhoneCall(action) {
			t.Fatalf("video=%v should be an answered call", video)
		}
		messagePts := int32(90)
		read := answeredCallReadUpdate(11, 44, 3, messagePts+1)
		message := mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Pts_INT32: messagePts,
			PtsCount:  1,
		}).To_Update()
		updates := mtproto.MakeUpdatesByUpdates(message)
		updates.PushBackUpdate(read)

		if len(updates.Updates) != 2 {
			t.Fatalf("updates %d", len(updates.Updates))
		}
		if updates.Updates[0].GetPts_INT32() != messagePts || updates.Updates[1].GetPts_INT32() != messagePts+1 {
			t.Fatalf("pts order %d then %d", updates.Updates[0].GetPts_INT32(), updates.Updates[1].GetPts_INT32())
		}
		if read.GetPredicateName() != mtproto.Predicate_updateReadHistoryInbox {
			t.Fatalf("update %s", read.GetPredicateName())
		}
		if read.GetPeer_PEER().GetUserId() != 11 || read.GetMaxId() != 44 || read.GetStillUnreadCount() != 3 || read.GetPtsCount() != 1 {
			t.Fatalf("update %+v", read)
		}
		// A read pushed in front of the message would deliver the higher pts first.
		front := mtproto.MakeUpdatesByUpdates(message)
		front.PushFrontUpdate(read)
		if front.Updates[0].GetPts_INT32() <= front.Updates[1].GetPts_INT32() {
			t.Fatal("fixture no longer demonstrates an out-of-order front insert")
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
