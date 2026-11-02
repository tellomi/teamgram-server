package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestBadEntitiesDoNotBreakGetDifference(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	c, cancel := ctx()
	defer cancel()
	ptsA, qtsA, dateA, err := a.State(c)
	if err != nil {
		t.Fatal(err)
	}
	ptsB, qtsB, dateB, err := b.State(c)
	if err != nil {
		t.Fatal(err)
	}
	text := "hello"
	entities := []*mtproto.MessageEntity{
		entityBold(0, 0),
		entityBold(0, 0),
		entityBold(0, int32(len(text))),
		entityBold(0, int32(len(text))),
	}
	if _, err := a.SendText(c, inputPeerUser(bSelf.ID, bSelf.AccessHash), text, entities); err != nil {
		t.Fatal(err)
	}
	for _, side := range []struct {
		name     string
		client   *Client
		pts, qts int32
		date     int32
		peer     *mtproto.InputPeer
	}{
		{"sender", a, ptsA, qtsA, dateA, inputPeerUser(bSelf.ID, bSelf.AccessHash)},
		{"recipient", b, ptsB, qtsB, dateB, inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)},
	} {
		t.Run(side.name, func(t *testing.T) {
			c, cancel := contextTimeout(45 * time.Second)
			defer cancel()
			diff, err := side.client.Difference(c, side.pts, side.qts, side.date)
			if err != nil {
				t.Fatalf("updates.getDifference: %v", err)
			}
			switch diff.(type) {
			case *mtproto.TLUpdatesDifference, *mtproto.TLUpdatesDifferenceSlice, *mtproto.TLUpdatesDifferenceEmpty, *mtproto.TLUpdatesDifferenceTooLong:
			default:
				t.Fatalf("updates.getDifference returned %T", diff)
			}
			err = waitUntil(c, 400*time.Millisecond, func() (bool, error) {
				msgs, err := side.client.History(c, side.peer, 30)
				if err != nil {
					return false, err
				}
				return hasText(msgs, text), nil
			})
			if err != nil {
				t.Fatalf("history never showed %q: %v", text, err)
			}
		})
	}
}
