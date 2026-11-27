package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestToggleSlowModeSeconds is NOTE-S-SLOWMODE-SECONDS-001 (问题清单行 114).
// Official steps are 0, 10, 30, 60, 300, 900, and 3600. 5, 7, 15, 20, and
// 120 return 400 SECONDS_INVALID and leave the stored duration alone.
// Phone 9996610942, code 11111, layer 232.
func TestToggleSlowModeSeconds(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610942", "SlowSec")
	chID, hash, err := c.CreateMegagroup(ctx, fmt.Sprintf("SlowSec-%d", time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, hash)
	t.Cleanup(func() {
		cctx, cancel := contextTimeout(20 * time.Second)
		defer cancel()
		_, _ = c.Invoke(cctx, &mtproto.TLChannelsDeleteChannel{
			Constructor: mtproto.CRC32_channels_deleteChannel,
			Channel:     channel,
		})
	})

	for _, sec := range []int32{5, 7, 15, 20, 120} {
		_, err := c.Invoke(ctx, toggleSlowMode(channel, sec))
		if err == nil || !strings.Contains(err.Error(), "SECONDS_INVALID") {
			t.Fatalf("seconds %d: %v", sec, err)
		}
		if got := slowmodeSeconds(t, ctx, c, channel); got != 0 {
			t.Fatalf("rejected %d stored %d", sec, got)
		}
	}
	for _, sec := range []int32{10, 30, 60, 300, 900, 3600, 0} {
		if _, err := c.Invoke(ctx, toggleSlowMode(channel, sec)); err != nil {
			t.Fatalf("seconds %d: %v", sec, err)
		}
		if got := slowmodeSeconds(t, ctx, c, channel); got != sec {
			t.Fatalf("stored %d want %d", got, sec)
		}
	}
	if _, err := c.Invoke(ctx, toggleSlowMode(channel, 10)); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Invoke(ctx, toggleSlowMode(channel, 15)); err == nil || !strings.Contains(err.Error(), "SECONDS_INVALID") {
		t.Fatalf("15 after 10: %v", err)
	}
	if got := slowmodeSeconds(t, ctx, c, channel); got != 10 {
		t.Fatalf("15 replaced 10 with %d", got)
	}
}

func toggleSlowMode(channel *mtproto.InputChannel, seconds int32) *mtproto.TLChannelsToggleSlowMode {
	return &mtproto.TLChannelsToggleSlowMode{
		Constructor: mtproto.CRC32_channels_toggleSlowMode,
		Channel:     channel,
		Seconds:     seconds,
	}
}

func slowmodeSeconds(t *testing.T, ctx context.Context, c *Client, channel *mtproto.InputChannel) int32 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     channel,
	})
	if err != nil {
		t.Fatal(err)
	}
	full := asChatFull(t, obj)
	if full.GetSlowmodeSeconds() == nil {
		return 0
	}
	return full.GetSlowmodeSeconds().GetValue()
}
