package dao

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/historyttl"
)

// TestStampHistoryTTLIgnoresStaleDialogCache is the send-path counterpart
// of the getDialogs overlay: Redis still holds DialogExt.TtlPeriod=0, but
// dialogs.ttl_period is already 86400. messages.sendMessage / sendMedia /
// forwardMessages (and the inbox rewrite of the peer's box) must stamp
// both the outbox and the peer inbox from the column, not the cache.
func TestStampHistoryTTLIgnoresStaleDialogCache(t *testing.T) {
	const staleCacheTTL int32 = 0
	d := &Dao{
		historyTTLQuery: func(ctx context.Context, query string, args ...interface{}) int32 {
			// The production lookup never reads DialogExt. A test that
			// started consulting staleCacheTTL would fail the asserts
			// below because that value is 0.
			_ = staleCacheTTL
			if !strings.Contains(query, "from dialogs") {
				return 0
			}
			if len(args) < 2 {
				return 0
			}
			userID, _ := args[0].(int64)
			peerID, _ := args[1].(int64)
			if (userID == 10 && peerID == 20) || (userID == 20 && peerID == 10) {
				return 86400
			}
			return 0
		},
	}

	outMsg := mtproto.MakeTLMessage(&mtproto.Message{Message: "hi"}).To_Message()
	inMsg := mtproto.MakeTLMessage(&mtproto.Message{Message: "hi"}).To_Message()
	d.stampHistoryTTL(context.Background(), 10, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: 20}, outMsg)
	d.stampHistoryTTL(context.Background(), 20, &mtproto.PeerUtil{PeerType: mtproto.PEER_USER, PeerId: 10}, inMsg)

	if got := historyttl.PeriodOf(outMsg); got != 86400 {
		t.Fatalf("outbox ttl_period=%d, want 86400 (stale cache was %d)", got, staleCacheTTL)
	}
	if got := historyttl.PeriodOf(inMsg); got != 86400 {
		t.Fatalf("peer inbox ttl_period=%d, want 86400 (stale cache was %d)", got, staleCacheTTL)
	}
}

func TestHistoryTTLForBasicGroupReadsChatsColumn(t *testing.T) {
	d := &Dao{
		historyTTLQuery: func(ctx context.Context, query string, args ...interface{}) int32 {
			if strings.Contains(query, "from chats") {
				return 86400
			}
			// A stale dialog row or cache at 0 must not hide the chat timer.
			return 0
		},
	}
	if got := d.historyTTL(context.Background(), 10, &mtproto.PeerUtil{PeerType: mtproto.PEER_CHAT, PeerId: 30}); got != 86400 {
		t.Fatalf("chat ttl_period=%d, want 86400 from chats", got)
	}
}
