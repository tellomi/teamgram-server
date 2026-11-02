package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestReactionPushedToPeer checks the other side of a private chat is told
// right away when a reaction is added or removed, on its own message id.
// https://core.telegram.org/constructor/updateMessageReactions
func TestReactionPushedToPeer(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	c, cancel := contextTimeout(60 * time.Second)
	defer cancel()
	if _, _, _, err := a.State(c); err != nil {
		t.Fatal(err)
	}
	if _, err := a.SendText(c, inputPeerSelf(), fmt.Sprintf("push-skew-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("push-react-%d", time.Now().UnixNano())
	aID, err := a.SendText(c, inputPeerUser(bSelf.ID, bSelf.AccessHash), text, nil)
	if err != nil {
		t.Fatal(err)
	}
	peerForB := inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)
	var bID int32
	if err := waitUntil(c, 400*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(c, peerForB, 30)
		if err != nil {
			return false, err
		}
		bID = idOfText(msgs, text)
		return bID != 0, nil
	}); err != nil {
		t.Fatalf("recipient never got %q: %v", text, err)
	}

	if _, err := b.SendReaction(c, peerForB, bID, "👍"); err != nil {
		t.Fatal(err)
	}
	if err := waitUntil(c, 300*time.Millisecond, func() (bool, error) {
		return pushedReaction(a.Pushed(), aID, func(r *mtproto.MessageReactions) bool {
			m := mtproto.MakeTLMessage(&mtproto.Message{Id: aID, Reactions: r}).To_Message()
			return hasReaction([]*mtproto.Message{m}, aID, "👍")
		}), nil
	}); err != nil {
		t.Fatalf("sender was not pushed updateMessageReactions 👍 on its id %d (recipient id %d): %v", aID, bID, err)
	}

	if _, err := b.Invoke(c, &mtproto.TLMessagesSendReaction{
		Constructor: mtproto.CRC32_messages_sendReaction_d30d78d4,
		Peer:        peerForB,
		MsgId:       bID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := waitUntil(c, 300*time.Millisecond, func() (bool, error) {
		return pushedReaction(a.Pushed(), aID, func(r *mtproto.MessageReactions) bool {
			return len(r.GetResults()) == 0
		}), nil
	}); err != nil {
		t.Fatalf("sender was not pushed the removal on %d: %v", aID, err)
	}
}

func pushedReaction(objs []mtproto.TLObject, msgID int32, ok func(*mtproto.MessageReactions) bool) bool {
	for _, obj := range objs {
		var ups []*mtproto.Update
		switch v := obj.(type) {
		case *mtproto.TLUpdates:
			ups = v.GetUpdates()
		case *mtproto.TLUpdatesCombined:
			ups = v.GetUpdates()
		case *mtproto.TLUpdateShort:
			ups = []*mtproto.Update{v.GetUpdate()}
		}
		for _, u := range ups {
			if u.GetPredicateName() == mtproto.Predicate_updateMessageReactions && u.GetMsgId_INT32() == msgID && ok(u.GetReactions_MESSAGEREACTIONS()) {
				return true
			}
		}
	}
	return false
}
