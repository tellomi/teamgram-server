package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestChannelDifferencePages is the client-visible check for
// updates.getChannelDifference. A backlog larger than limit comes back as
// updates.channelDifference with final unset, and the next call continues
// from that pts. channelDifferenceTooLong is not used while the pts log
// can still fill the gap.
func TestChannelDifferencePages(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610414", "ChanDiff")
	chID, hash, err := c.CreateMegagroup(ctx, fmt.Sprintf("diff-%d", time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	channel := inputChannel(chID, hash)
	peer := inputPeerChannel(chID, hash)

	caught, err := channelDifference(ctx, c, channel, 0, 100)
	if err != nil {
		t.Fatal(err)
	}
	create, ok := caught.(*mtproto.TLUpdatesChannelDifference)
	if !ok || !create.GetFinal() {
		t.Fatalf("create difference %T final", caught)
	}
	base := create.GetPts()
	if base == 0 {
		t.Fatal("create difference pts is 0")
	}

	const n = 40
	for i := 0; i < n; i++ {
		if _, err := c.SendText(ctx, peer, fmt.Sprintf("diff-%02d", i), nil); err != nil {
			t.Fatalf("send %d: %v", i, err)
		}
	}

	seen := map[int32]struct{}{}
	pts := base
	pages := 0
	for {
		pages++
		if pages > 10 {
			t.Fatal("difference did not finish")
		}
		res, err := channelDifference(ctx, c, channel, pts, 10)
		if err != nil {
			t.Fatal(err)
		}
		switch diff := res.(type) {
		case *mtproto.TLUpdatesChannelDifferenceTooLong:
			t.Fatalf("tooLong from pts %d on page %d", pts, pages)
		case *mtproto.TLUpdatesChannelDifferenceEmpty:
			if !diff.GetFinal() || diff.GetPts() < pts {
				t.Fatalf("empty final=%v pts=%d", diff.GetFinal(), diff.GetPts())
			}
			pts = diff.GetPts()
		case *mtproto.TLUpdatesChannelDifference:
			if diff.GetPts() <= pts {
				t.Fatalf("pts %d did not advance from %d", diff.GetPts(), pts)
			}
			if len(diff.GetNewMessages()) > 10 {
				t.Fatalf("page %d", len(diff.GetNewMessages()))
			}
			for _, m := range diff.GetNewMessages() {
				if m.GetId() == 0 {
					continue
				}
				if _, dup := seen[m.GetId()]; dup {
					t.Fatalf("duplicate %d", m.GetId())
				}
				seen[m.GetId()] = struct{}{}
			}
			pts = diff.GetPts()
			if !diff.GetFinal() {
				continue
			}
		default:
			t.Fatalf("difference %T", res)
		}
		break
	}
	if pages < 4 {
		t.Fatalf("pages %d for %d messages at limit 10", pages, n)
	}
	if len(seen) < n {
		t.Fatalf("delivered %d, sent %d", len(seen), n)
	}
	again, err := channelDifference(ctx, c, channel, pts, 10)
	if err != nil {
		t.Fatal(err)
	}
	empty, ok := again.(*mtproto.TLUpdatesChannelDifferenceEmpty)
	if !ok || !empty.GetFinal() || empty.GetPts() != pts {
		t.Fatalf("caught up %T", again)
	}

	one, err := channelDifference(ctx, c, channel, base, 1)
	if err != nil {
		t.Fatal(err)
	}
	page, ok := one.(*mtproto.TLUpdatesChannelDifference)
	if !ok || page.GetFinal() || len(page.GetNewMessages()) != 1 || page.GetPts() <= base {
		t.Fatalf("limit 1 %T messages=%d", one, len(messagesOfDifference(one)))
	}
}

func channelDifference(ctx context.Context, c *Client, channel *mtproto.InputChannel, pts, limit int32) (mtproto.TLObject, error) {
	return c.Invoke(ctx, &mtproto.TLUpdatesGetChannelDifference{
		Constructor: mtproto.CRC32_updates_getChannelDifference,
		Channel:     channel,
		Filter:      mtproto.MakeTLChannelMessagesFilterEmpty(nil).To_ChannelMessagesFilter(),
		Pts:         pts,
		Limit:       limit,
	})
}

func messagesOfDifference(obj mtproto.TLObject) []*mtproto.Message {
	switch diff := obj.(type) {
	case *mtproto.TLUpdatesChannelDifference:
		return diff.GetNewMessages()
	case *mtproto.TLUpdatesChannelDifferenceTooLong:
		return diff.GetMessages()
	default:
		return nil
	}
}
