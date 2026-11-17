package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestBlockedUserCannotReactInAPrivateChat is spreadsheet row 193.
// The person who was blocked gets 400 USER_IS_BLOCKED. The person who
// blocked the author gets 400 YOU_BLOCKED_USER. Neither reaction is stored
// or pushed. After both blocks are lifted, the same reaction is delivered.
func TestBlockedUserCannotReactInAPrivateChat(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "reactblk_a", "ReactA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "reactblk_b", "ReactB")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	peerForA := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	peerForB := inputPeerUser(aSelf.ID, aSelf.AccessHash)
	text := fmt.Sprintf("react-block-%d", time.Now().UnixNano())
	aID, err := a.SendText(ctx, peerForA, text, nil)
	if err != nil {
		t.Fatal(err)
	}
	var bID int32
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(ctx, peerForB, 30)
		if err != nil {
			return false, err
		}
		bID = idOfText(msgs, text)
		return bID != 0, nil
	}); err != nil {
		t.Fatalf("recipient never got %q: %v", text, err)
	}

	t.Cleanup(func() {
		clean, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		setBlocked(t, clean, a, peerForA, false)
		setBlocked(t, clean, b, peerForB, false)
	})

	setBlocked(t, ctx, a, peerForA, true)
	before := len(a.Pushed())
	_, err = b.SendReaction(ctx, peerForB, bID, "👍")
	wantRPC(t, err, 400, "USER_IS_BLOCKED")
	if reactionVisible(t, ctx, a, peerForA, aID) || reactionVisible(t, ctx, b, peerForB, bID) {
		t.Fatal("USER_IS_BLOCKED still left a reaction on the message")
	}
	if reactionWasPushed(a.Pushed()[before:]) {
		t.Fatal("the blocker was pushed the reaction")
	}

	setBlocked(t, ctx, a, peerForA, false)
	setBlocked(t, ctx, b, peerForB, true)
	before = len(a.Pushed())
	_, err = b.SendReaction(ctx, peerForB, bID, "👍")
	wantRPC(t, err, 400, "YOU_BLOCKED_USER")
	if reactionVisible(t, ctx, a, peerForA, aID) || reactionVisible(t, ctx, b, peerForB, bID) {
		t.Fatal("YOU_BLOCKED_USER still left a reaction on the message")
	}
	if reactionWasPushed(a.Pushed()[before:]) {
		t.Fatal("the author was pushed a reaction from someone who blocked them")
	}

	setBlocked(t, ctx, b, peerForB, false)
	res, err := b.SendReaction(ctx, peerForB, bID, "👍")
	if err != nil {
		t.Fatal(err)
	}
	if !reactionIn(res, bID, "👍") && !updateReactionOn(res, bID, "👍") {
		t.Fatalf("open chat did not accept 👍: %T", res)
	}
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		return reactionVisible(t, ctx, a, peerForA, aID), nil
	}); err != nil {
		t.Fatal("author never saw the reaction after the block was lifted")
	}
}

func setBlocked(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, blocked bool) {
	t.Helper()
	req := mtproto.TLObject(&mtproto.TLContactsUnblock{
		Constructor: mtproto.CRC32_contacts_unblock_b550d328,
		Id:          peer,
	})
	if blocked {
		req = &mtproto.TLContactsBlock{
			Constructor: mtproto.CRC32_contacts_block_2e2e8734,
			Id:          peer,
		}
	}
	if _, err := c.Invoke(ctx, req); err != nil {
		t.Fatal(err)
	}
}

func wantRPC(t *testing.T, err error, code int, message string) {
	t.Helper()
	rpc, ok := asRPCError(err)
	if !ok || rpc.Code != code || rpc.Message != message {
		t.Fatalf("got %v, want rpc %d %s", err, code, message)
	}
}

func reactionVisible(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, msgID int32) bool {
	t.Helper()
	msgs, err := c.History(ctx, peer, 30)
	if err != nil {
		t.Fatal(err)
	}
	return hasReaction(msgs, msgID, "👍")
}

func reactionWasPushed(objs []mtproto.TLObject) bool {
	for _, obj := range objs {
		var ups []*mtproto.Update
		switch v := obj.(type) {
		case *mtproto.TLUpdates:
			ups = v.GetUpdates()
		case *mtproto.TLUpdatesCombined:
			ups = v.GetUpdates()
		case *mtproto.TLUpdateShort:
			if v.GetUpdate() != nil {
				ups = []*mtproto.Update{v.GetUpdate()}
			}
		}
		for _, u := range ups {
			if u.GetPredicateName() == mtproto.Predicate_updateMessageReactions {
				return true
			}
		}
	}
	return false
}
