package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestHistoryTTLDeletesBothSides sets a one-second auto-delete timer, then
// checks that a message sent after the timer is removed from both clouds and
// that a message sent before it stays. The worker also pushes
// updateDeleteMessages, which getDifference returns with a pts.
func TestHistoryTTLDeletesBothSides(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	peerForA := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	peerForB := inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)

	keep := fmt.Sprintf("ttl-keep-%d", time.Now().UnixNano())
	if _, err := a.SendText(ctx, peerForA, keep, nil); err != nil {
		t.Fatal(err)
	}
	if err := setHistoryTTL(ctx, a, peerForA, 1); err != nil {
		t.Fatal(err)
	}
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		d, ok, err := dialogOf(a, ctx, "user", bSelf.ID)
		if err != nil || !ok {
			return false, err
		}
		return d.GetTtlPeriod().GetValue() == 1, nil
	}); err != nil {
		t.Fatal(err)
	}

	ptsA, qtsA, dateA, err := a.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ptsB, qtsB, dateB, err := b.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	drop := fmt.Sprintf("ttl-drop-%d", time.Now().UnixNano())
	if _, err := a.SendText(ctx, peerForA, drop, nil); err != nil {
		t.Fatal(err)
	}
	var idA, idB int32
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := a.History(ctx, peerForA, 30)
		if err != nil {
			return false, err
		}
		idA = idOfText(msgs, drop)
		if idA == 0 {
			return false, nil
		}
		for _, msg := range msgs {
			if msg.GetId() == idA && msg.GetTtlPeriod().GetValue() != 1 {
				return false, fmt.Errorf("sender ttl_period = %d", msg.GetTtlPeriod().GetValue())
			}
		}
		msgs, err = b.History(ctx, peerForB, 30)
		if err != nil {
			return false, err
		}
		idB = idOfText(msgs, drop)
		return idB != 0, nil
	}); err != nil {
		t.Fatal(err)
	}

	var gone bool
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		leftA, err := a.History(ctx, peerForA, 40)
		if err != nil {
			t.Fatal(err)
		}
		leftB, err := b.History(ctx, peerForB, 40)
		if err != nil {
			t.Fatal(err)
		}
		if idOfText(leftA, drop) == 0 && idOfText(leftB, drop) == 0 && idOfText(leftA, keep) != 0 && idOfText(leftB, keep) != 0 {
			gone = true
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	if !gone {
		t.Fatal("drop stayed or keep disappeared")
	}

	diffA, err := a.Difference(ctx, ptsA, qtsA, dateA)
	if err != nil {
		t.Fatal(err)
	}
	if !differenceDeletes(diffA, idA) {
		t.Fatalf("sender getDifference missing updateDeleteMessages for %d", idA)
	}
	diffB, err := b.Difference(ctx, ptsB, qtsB, dateB)
	if err != nil {
		t.Fatal(err)
	}
	if !differenceDeletes(diffB, idB) {
		t.Fatalf("recipient getDifference missing updateDeleteMessages for %d", idB)
	}
}

func TestHistoryTTLGroupAndChannel(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	chatID, err := a.CreateChat(ctx, fmt.Sprintf("ttl-group-%d", time.Now().UnixNano()), inputUser(bSelf.ID, bSelf.AccessHash))
	if err != nil {
		t.Fatal(err)
	}
	group := inputPeerChat(chatID)
	if err := setHistoryTTL(ctx, a, group, 1); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("ttl-group-msg-%d", time.Now().UnixNano())
	if _, err := a.SendText(ctx, group, text, nil); err != nil {
		t.Fatal(err)
	}
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		left, err := b.History(ctx, group, 20)
		if err != nil {
			return false, err
		}
		id := idOfText(left, text)
		if id == 0 {
			return false, nil
		}
		for _, msg := range left {
			if msg.GetId() == id && msg.GetTtlPeriod().GetValue() != 1 {
				return false, fmt.Errorf("group ttl_period = %d", msg.GetTtlPeriod().GetValue())
			}
		}
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := waitUntil(ctx, 500*time.Millisecond, func() (bool, error) {
		left, err := b.History(ctx, group, 20)
		if err != nil {
			return false, err
		}
		return idOfText(left, text) == 0, nil
	}); err != nil {
		t.Fatalf("group message was not deleted for the other member: %v", err)
	}

	chID, chHash, err := a.CreateMegagroup(ctx, fmt.Sprintf("ttl-mega-%d", time.Now().UnixNano()))
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, chHash)
	if err := a.InviteToChannel(ctx, channel, inputUser(bSelf.ID, bSelf.AccessHash)); err != nil {
		t.Fatal(err)
	}
	if err := setHistoryTTL(ctx, a, inputPeerChannel(chID, chHash), 1); err != nil {
		t.Fatal(err)
	}
	post := fmt.Sprintf("ttl-channel-msg-%d", time.Now().UnixNano())
	if _, err := a.SendText(ctx, inputPeerChannel(chID, chHash), post, nil); err != nil {
		t.Fatal(err)
	}
	var bobHash int64
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		got, ok, err := b.ChatAccess(ctx, chID)
		if err != nil || !ok {
			return false, err
		}
		bobHash = got
		left, err := b.History(ctx, inputPeerChannel(chID, bobHash), 20)
		if err != nil {
			return false, err
		}
		return idOfText(left, post) != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := waitUntil(ctx, 500*time.Millisecond, func() (bool, error) {
		left, err := b.History(ctx, inputPeerChannel(chID, bobHash), 20)
		if err != nil {
			return false, err
		}
		return idOfText(left, post) == 0, nil
	}); err != nil {
		t.Fatalf("channel message was not deleted: %v", err)
	}
}

func setHistoryTTL(ctx context.Context, c *Client, peer *mtproto.InputPeer, period int32) error {
	_, err := c.Invoke(ctx, &mtproto.TLMessagesSetHistoryTTL{
		Constructor: mtproto.CRC32_messages_setHistoryTTL,
		Peer:        peer,
		Period:      period,
	})
	return err
}

func dialogOf(c *Client, ctx context.Context, kind string, peerID int64) (*mtproto.Dialog, bool, error) {
	dialogs, err := c.Dialogs(ctx)
	if err != nil {
		return nil, false, err
	}
	for _, d := range dialogs {
		if d == nil || d.GetPeer() == nil {
			continue
		}
		var id int64
		switch kind {
		case "user":
			id = d.GetPeer().GetUserId()
		case "chat":
			id = d.GetPeer().GetChatId()
		case "channel":
			id = d.GetPeer().GetChannelId()
		}
		if id == peerID {
			return d, true, nil
		}
	}
	return nil, false, nil
}

func differenceDeletes(obj mtproto.TLObject, messageID int32) bool {
	for _, up := range differenceUpdates(obj) {
		if up.GetPredicateName() != mtproto.Predicate_updateDeleteMessages && up.GetPredicateName() != mtproto.Predicate_updateDeleteChannelMessages {
			continue
		}
		if up.GetPts_INT32() == 0 || up.GetPtsCount() == 0 {
			continue
		}
		for _, id := range up.GetMessages() {
			if id == messageID {
				return true
			}
		}
	}
	return false
}

func differenceUpdates(obj mtproto.TLObject) []*mtproto.Update {
	switch v := obj.(type) {
	case *mtproto.TLUpdatesDifference:
		return v.GetOtherUpdates()
	case *mtproto.TLUpdatesDifferenceSlice:
		return v.GetOtherUpdates()
	default:
		return nil
	}
}
