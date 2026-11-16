package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
)

const (
	retrySender int64 = 910442001
	retryPeer   int64 = 910442002
	retryRandom int64 = 910442003
)

func TestFinishReplayPublishesLiveAndSkipsDeleted(t *testing.T) {
	live := &mtproto.MessageBox{MessageId: 7, RandomId: retryRandom, UserId: retrySender, Pts: 4, PtsCount: 1, Message: mtproto.MakeTLMessage(&mtproto.Message{Out: true, Id: 7, Message: "kept"}).To_Message()}
	published := 0
	done, ups, err := finishReplay(dao.ReplayLive, []*mtproto.MessageBox{live}, []*mtproto.MessageBox{live}, func(boxes []*mtproto.MessageBox) error {
		published++
		if len(boxes) != 1 || boxes[0].MessageId != 7 {
			t.Fatalf("published %#v", boxes)
		}
		return nil
	}, func(all []*mtproto.MessageBox) *mtproto.Updates {
		return updatesForBoxes(all, func([]int64) []*mtproto.User { return nil }, func([]int64) []*mtproto.Chat { return nil }, func([]int64) []*mtproto.Chat { return nil })
	})
	if err != nil || !done || published != 1 {
		t.Fatalf("live done=%v published=%d err=%v", done, published, err)
	}
	var gotPts int32
	for _, upd := range ups.GetUpdates() {
		if upd.GetPredicateName() == mtproto.Predicate_updateNewMessage {
			gotPts = upd.GetPts_INT32()
		}
	}
	if gotPts != 4 {
		t.Fatalf("sender update pts=%d ups=%v", gotPts, ups)
	}

	published = 0
	done, _, err = finishReplay(dao.ReplayLive, []*mtproto.MessageBox{live}, []*mtproto.MessageBox{live}, func([]*mtproto.MessageBox) error {
		published++
		return errors.New("kafka down")
	}, func([]*mtproto.MessageBox) *mtproto.Updates { return nil })
	if err == nil || !done || published != 1 {
		t.Fatalf("failed publish done=%v published=%d err=%v", done, published, err)
	}

	published = 0
	deleted := &mtproto.MessageBox{MessageId: 7, Message: mtproto.MakeTLMessage(&mtproto.Message{Id: 7, Message: ""}).To_Message()}
	done, ups, err = finishReplay(dao.ReplayDeleted, nil, []*mtproto.MessageBox{deleted}, func([]*mtproto.MessageBox) error {
		published++
		return nil
	}, func(all []*mtproto.MessageBox) *mtproto.Updates {
		return updatesForBoxes(all, func([]int64) []*mtproto.User { return nil }, func([]int64) []*mtproto.Chat { return nil }, func([]int64) []*mtproto.Chat { return nil })
	})
	if err != nil || !done || published != 0 {
		t.Fatalf("deleted replay published=%d done=%v err=%v", published, done, err)
	}
	body := ""
	found := false
	for _, upd := range ups.GetUpdates() {
		if upd.GetPredicateName() == mtproto.Predicate_updateNewMessage {
			found = true
			body = upd.GetMessage_MESSAGE().GetMessage()
		}
	}
	if !found || body != "" {
		t.Fatalf("deleted replay body %q found %v", body, found)
	}

	done, _, err = finishReplay(dao.ReplayMissing, nil, nil, func([]*mtproto.MessageBox) error {
		t.Fatal("missing row published")
		return nil
	}, func([]*mtproto.MessageBox) *mtproto.Updates { return nil })
	if err != nil || done {
		t.Fatalf("missing done=%v err=%v", done, err)
	}
}

func TestPrivateRetryReachesRecipientOnce(t *testing.T) {
	box := &mtproto.MessageBox{MessageId: 7, RandomId: retryRandom, UserId: retrySender, Pts: 4, PtsCount: 1}
	reqs := privateInboxReqs(retrySender, retryPeer, 910442004, []*mtproto.MessageBox{box}, nil, nil, true)
	if len(reqs) != 2 {
		t.Fatalf("reqs = %d", len(reqs))
	}
	if !reqs[0].Out || reqs[0].UserId != retrySender || reqs[0].BoxList[0].Pts != 4 {
		t.Fatalf("sender copy %#v", reqs[0])
	}
	if reqs[1].Out || reqs[1].UserId != retryPeer || reqs[1].BoxList[0] != box {
		t.Fatalf("recipient copy %#v", reqs[1])
	}
	again := privateInboxReqs(retrySender, retryPeer, 910442004, []*mtproto.MessageBox{box}, nil, nil, true)
	if again[1].BoxList[0].MessageId != reqs[1].BoxList[0].MessageId {
		t.Fatal("retry allocated another message id")
	}
	if len(privateInboxReqs(retrySender, retryPeer, 910442004, []*mtproto.MessageBox{box}, nil, nil, false)) != 1 {
		t.Fatal("a blocked recipient was given a copy")
	}
}

func TestSyncUpdatesNotMeKeepsStoredPts(t *testing.T) {
	// The consumer pushes updateNewMessage with the pts already stored for
	// this box. A device whose local pts is still one behind applies it.
	// A device that has moved on does not, because localPts + pts_count is
	// not equal to this older pts. The pts row itself is not written again
	// on the sender replay path.
	const pts, count int32 = 4, 1
	upd := outgoingUpdate(&mtproto.MessageBox{
		MessageId: 7,
		RandomId:  retryRandom,
		Pts:       pts,
		PtsCount:  count,
		Message:   mtproto.MakeTLMessage(&mtproto.Message{Out: true, Id: 7, Message: "kept"}).To_Message(),
	})
	if upd.GetPts_INT32() != 4 || upd.GetPtsCount() != 1 {
		t.Fatalf("other-device update pts=%d count=%d", upd.GetPts_INT32(), upd.GetPtsCount())
	}
}
