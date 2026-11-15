package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestTranslateTextWithoutInputIs400 covers ISSUE-S-TRANSLATE-NIL-001 (row 268).
// messages.translateText with neither text nor peer/id used to panic and
// return RPC 500.
func TestTranslateTextWithoutInputIs400(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(60 * time.Second)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610447", "TrNil")
	_, err := c.Invoke(ctx, &mtproto.TLMessagesTranslateText{
		Constructor: mtproto.CRC32_messages_translateText_a5eec345,
		ToLang:      "zh",
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Code != 400 || rpc.Message != "PEER_ID_INVALID" {
		t.Fatalf("translateText with no input: %v", err)
	}
}
