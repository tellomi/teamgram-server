package e2e

import (
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestPinnedStateDifferenceCoversLaterMessages checks the peerDialogs
// cursor. A message sent after messages.getPinnedDialogs must show up in
// updates.getDifference from that response's state.
func TestPinnedStateDifferenceCoversLaterMessages(t *testing.T) {
	a, b, _, _ := sharedPair(t)
	ctx, cancel := contextTimeout(60 * time.Second)
	defer cancel()
	res, err := a.Invoke(ctx, &mtproto.TLMessagesGetPinnedDialogs{
		Constructor: mtproto.CRC32_messages_getPinnedDialogs,
	})
	if err != nil {
		t.Fatal(err)
	}
	peers, ok := res.(*mtproto.TLMessagesPeerDialogs)
	if !ok || peers.GetState() == nil {
		t.Fatalf("messages.getPinnedDialogs returned %T", res)
	}
	state := peers.GetState()
	text := fmt.Sprintf("pinned-state-%d", time.Now().UnixNano())
	if _, err = b.SendText(ctx, inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash), text, nil); err != nil {
		t.Fatal(err)
	}
	err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		diff, err := a.Difference(ctx, state.GetPts(), state.GetQts(), state.GetDate())
		if err != nil {
			return false, err
		}
		if _, tooLong := diff.(*mtproto.TLUpdatesDifferenceTooLong); tooLong {
			return false, fmt.Errorf("differenceTooLong from pinned state pts %d", state.GetPts())
		}
		return hasText(allMessages(diff), text), nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
