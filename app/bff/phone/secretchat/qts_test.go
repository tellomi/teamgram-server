package secretchat

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func input(chat *mtproto.EncryptedChat) *mtproto.InputEncryptedChat {
	return mtproto.MakeTLInputEncryptedChat(&mtproto.InputEncryptedChat{
		ChatId: chat.GetId(), AccessHash: chat.GetAccessHash(),
	}).To_InputEncryptedChat()
}

func TestOfflineRecipientCatchesTheInvite(t *testing.T) {
	be := newMem()
	admin := New(be)
	waiting, events, err := admin.Request(1, 2, 42, []byte{9}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].UserID != 2 {
		t.Fatalf("events %+v", events)
	}
	// The participant is offline: the live push is not applied. The qts
	// queue is what they read on the next getDifference.
	msgs, others, last, more := admin.Gap(2, 0, 10)
	if more || last != 1 || len(msgs) != 0 || len(others) != 1 {
		t.Fatalf("gap msgs=%d others=%d last=%d more=%v", len(msgs), len(others), last, more)
	}
	req := others[0].GetChat()
	if req.GetPredicateName() != mtproto.Predicate_encryptedChatRequested || req.GetId() != 42 || string(req.GetGA()) != string([]byte{9}) {
		t.Fatalf("invite %+v", req)
	}
	peer := New(be)
	accepted, _, err := peer.Accept(2, input(waiting), []byte{8}, 99, 11)
	if err != nil {
		t.Fatal(err)
	}
	if accepted.GetPredicateName() != mtproto.Predicate_encryptedChat || string(accepted.GetGAOrB()) != string([]byte{9}) {
		t.Fatalf("accepted %+v", accepted)
	}
}

func TestOldClientLocalQtsZeroDoesNotSpin(t *testing.T) {
	Reset()
	t.Cleanup(Reset)

	empty := mtproto.MakeTLUpdatesDifferenceEmpty(&mtproto.Updates_Difference{
		Date: 10, Seq: 1,
	}).To_Updates_Difference()
	if Current(2) != 0 {
		t.Fatal("getState qts")
	}
	idle := ApplyDifference(2, 0, 100, empty)
	if idle.GetPredicateName() != mtproto.Predicate_updates_differenceEmpty {
		t.Fatalf("client with no secret updates got %s", idle.GetPredicateName())
	}

	waiting, _, err := Default().Request(1, 2, 42, []byte{9}, 10)
	if err != nil {
		t.Fatal(err)
	}
	peer := input(waiting)
	if _, _, err = Default().Accept(2, peer, []byte{8}, 3, 11); err != nil {
		t.Fatal(err)
	}
	if _, _, err = Default().Send(1, peer, 100, []byte("cipher-text"), nil, false, 12); err != nil {
		t.Fatal(err)
	}
	serverQts := Current(2)
	if serverQts == 0 {
		t.Fatal("getState qts stayed 0 after a secret update")
	}

	local := int32(0)
	var sawCipher bool
	for step := 0; step < 8; step++ {
		page := ApplyDifference(2, local, 1, empty)
		switch page.GetPredicateName() {
		case mtproto.Predicate_updates_differenceSlice:
			next := page.GetIntermediateState().GetQts()
			if next <= local {
				t.Fatalf("slice qts %d did not pass local %d", next, local)
			}
			for _, upd := range page.GetOtherUpdates() {
				if upd.GetQts() != 0 {
					t.Fatalf("other_updates carried qts %d", upd.GetQts())
				}
			}
			if len(page.GetNewEncryptedMessages()) > 0 {
				sawCipher = true
			}
			local = next
		case mtproto.Predicate_updates_difference:
			next := page.GetState().GetQts()
			if next != serverQts {
				t.Fatalf("final qts %d, getState %d", next, serverQts)
			}
			for _, upd := range page.GetOtherUpdates() {
				if upd.GetQts() != 0 {
					t.Fatalf("other_updates carried qts %d", upd.GetQts())
				}
			}
			if len(page.GetNewEncryptedMessages()) > 0 {
				sawCipher = true
			}
			local = next
			again := ApplyDifference(2, local, 100, empty)
			if again.GetPredicateName() != mtproto.Predicate_updates_differenceEmpty {
				t.Fatalf("caught-up getDifference returned %s", again.GetPredicateName())
			}
			if !sawCipher {
				t.Fatal("qts=0 client never received the encrypted message")
			}
			return
		default:
			t.Fatalf("getDifference returned %s", page.GetPredicateName())
		}
	}
	t.Fatal("getDifference did not finish")
}

func TestApplyDifferenceReplacesEmpty(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	if _, _, err := Default().Request(1, 2, 42, []byte{9}, 10); err != nil {
		t.Fatal(err)
	}
	empty := mtproto.MakeTLUpdatesDifferenceEmpty(&mtproto.Updates_Difference{Date: 11, Seq: 3}).To_Updates_Difference()
	diff := ApplyDifference(2, 0, 10, empty)
	if diff.GetPredicateName() == mtproto.Predicate_updates_differenceEmpty || diff.GetState().GetQts() == 0 {
		t.Fatalf("difference %s qts=%d", diff.GetPredicateName(), diff.GetState().GetQts())
	}
	if diff.OtherUpdates[0].GetChat().GetPredicateName() != mtproto.Predicate_encryptedChatRequested {
		t.Fatal("difference dropped the invite")
	}
	caughtUp := ApplyDifference(2, diff.GetState().GetQts(), 10, empty)
	if caughtUp.GetPredicateName() != mtproto.Predicate_updates_differenceEmpty {
		t.Fatalf("caught up should stay empty, got %s", caughtUp.GetPredicateName())
	}
}

func TestRestartKeepsTheWaitingChat(t *testing.T) {
	be := newMem()
	first := New(be)
	waiting, _, err := first.Request(1, 2, 7, []byte{1, 2}, 10)
	if err != nil {
		t.Fatal(err)
	}
	// The first process is gone. The backend still has the row.
	second := New(be)
	if _, _, err := second.Accept(2, input(waiting), []byte{3}, 4, 12); err != nil {
		t.Fatal(err)
	}
	if Current := second.be.Current(1); Current == 0 {
		t.Fatal("admin qts was not persisted for the accept update")
	}
}

func TestTwoInstancesShareOneBackend(t *testing.T) {
	be := newMem()
	a := New(be)
	b := New(be)
	waiting, _, err := a.Request(1, 2, 15, []byte{4}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Accept(2, input(waiting), []byte{5}, 6, 11); err != nil {
		t.Fatal(err)
	}
	sent, events, err := a.Send(1, input(waiting), 100, []byte("cipher-text"), nil, false, 12)
	if err != nil || sent.GetDate() != 12 {
		t.Fatalf("send %+v %v", sent, err)
	}
	if events[0].Update.GetQts() == 0 {
		t.Fatal("encrypted message was pushed with qts 0")
	}
	msgs, others, _, more := b.Gap(2, 0, 1)
	if !more {
		t.Fatal("qts limit did not leave a slice")
	}
	if len(msgs) != 0 || others[0].GetChat().GetPredicateName() != mtproto.Predicate_encryptedChatRequested {
		t.Fatal("first qts item should be the invite, not the ciphertext")
	}
	msgs, others, last, more := b.Gap(2, 1, 10)
	if more || last == 0 || len(msgs) != 1 || string(msgs[0].GetBytes()) != "cipher-text" {
		t.Fatalf("rest msgs=%d last=%d more=%v", len(msgs), last, more)
	}
	if len(others) != 0 {
		t.Fatal("encrypted message was also placed in other_updates")
	}
	if events[0].Update.GetQts() != last {
		t.Fatalf("live qts %d queue %d", events[0].Update.GetQts(), last)
	}
	// A third instance, after the first two are discarded, still reads the queue.
	c := New(be)
	if c.be.Current(2) != b.be.Current(2) {
		t.Fatal("qts cursor differs across instances")
	}
}
