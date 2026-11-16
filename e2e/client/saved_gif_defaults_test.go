package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSavedGifKeepsDefaults is 行 146 / ISSUE-S-GIF-DEFAULT-001.
// Saving one GIF must not drop the built-in panel.
// Phone 9996610659, code 11111.
func TestSavedGifKeepsDefaults(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610659", "GifSave")
	before := savedGifs(t, ctx, c)
	if len(before.GetGifs()) < 4 {
		t.Fatalf("defaults %d", len(before.GetGifs()))
	}
	doc := before.GetGifs()[0]
	if _, err := c.Invoke(ctx, &mtproto.TLMessagesSaveGif{
		Constructor: mtproto.CRC32_messages_saveGif,
		Id: mtproto.MakeTLInputDocument(&mtproto.InputDocument{
			Id:         doc.GetId(),
			AccessHash: doc.GetAccessHash(),
		}).To_InputDocument(),
		Unsave: mtproto.BoolFalse,
	}); err != nil {
		t.Fatal(err)
	}
	after := savedGifs(t, ctx, c)
	if len(after.GetGifs()) < len(before.GetGifs()) || after.GetGifs()[0].GetId() != doc.GetId() {
		t.Fatalf("after save %d, before %d, first %d", len(after.GetGifs()), len(before.GetGifs()), after.GetGifs()[0].GetId())
	}
	if _, err := c.Invoke(ctx, &mtproto.TLMessagesSaveGif{
		Constructor: mtproto.CRC32_messages_saveGif,
		Id: mtproto.MakeTLInputDocument(&mtproto.InputDocument{
			Id:         doc.GetId(),
			AccessHash: doc.GetAccessHash(),
		}).To_InputDocument(),
		Unsave: mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	cleared := savedGifs(t, ctx, c)
	if len(cleared.GetGifs()) < len(before.GetGifs()) {
		t.Fatalf("after unsave %d, before %d", len(cleared.GetGifs()), len(before.GetGifs()))
	}
}

func savedGifs(t *testing.T, ctx context.Context, c *Client) *mtproto.Messages_SavedGifs {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetSavedGifs{
		Constructor: mtproto.CRC32_messages_getSavedGifs,
		Hash:        0,
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesSavedGifs:
		return v.To_Messages_SavedGifs()
	default:
		t.Fatalf("getSavedGifs %T", obj)
		return nil
	}
}
