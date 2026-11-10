package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestFloodWaitOnImportContacts is the abuse bucket. An empty
// contacts.importContacts is cheap, and the limit is low enough that a
// short loop returns FLOOD_WAIT_X. updates.getState in the same session
// stays under the global bucket: the production probe of dozens of
// getState calls must not be a flood.
func TestFloodWaitOnImportContacts(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610097", "Flood")

	for i := 0; i < 70; i++ {
		if _, err := c.invokeOnce(ctx, &mtproto.TLUpdatesGetState{Constructor: mtproto.CRC32_updates_getState}); err != nil {
			t.Fatalf("getState %d: %v", i, err)
		}
	}

	req := &mtproto.TLContactsImportContacts{Constructor: mtproto.CRC32_contacts_importContacts}
	saw := false
	for i := 0; i < 40; i++ {
		_, err := c.invokeOnce(ctx, req)
		if err == nil {
			continue
		}
		rpc, ok := asRPCError(err)
		if !ok || rpc.Code != 420 || !strings.HasPrefix(rpc.Message, "FLOOD_WAIT_") {
			t.Fatalf("import %d: %v", i, err)
		}
		saw = true
		break
	}
	if !saw {
		t.Fatal("importContacts never returned FLOOD_WAIT")
	}
	// The global bucket is separate. A read after the import flood still works.
	if _, err := c.invokeOnce(ctx, &mtproto.TLUpdatesGetState{Constructor: mtproto.CRC32_updates_getState}); err != nil {
		t.Fatalf("getState after flood: %v", err)
	}
}
