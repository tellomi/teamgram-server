package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSavedAndUserDraftsBothReturn is messages.saveDraft then getAllDrafts.
// Saved Messages used to be stored as PEER_SELF, so the dialog UPDATE matched
// nothing and the later list omitted that draft.
func TestSavedAndUserDraftsBothReturn(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, aSelf := dialPhone(t, ctx, cfg, "9996610772", "DraftA")
	_, bSelf := dialPhone(t, ctx, cfg, "9996610773", "DraftB")

	if _, err := a.Invoke(ctx, &mtproto.TLMessagesSaveDraft{
		Constructor: mtproto.CRC32_messages_saveDraft_bc39e14b,
		Peer:        inputPeerSelf(),
		Message:     "saved note",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Invoke(ctx, &mtproto.TLMessagesSaveDraft{
		Constructor: mtproto.CRC32_messages_saveDraft_bc39e14b,
		Peer:        inputPeerUser(bSelf.ID, bSelf.AccessHash),
		Message:     "user note",
	}); err != nil {
		t.Fatal(err)
	}
	obj, err := a.Invoke(ctx, &mtproto.TLMessagesGetAllDrafts{
		Constructor: mtproto.CRC32_messages_getAllDrafts,
	})
	if err != nil {
		t.Fatal(err)
	}
	var saved, user bool
	for _, upd := range updatesOf(obj) {
		if upd.GetPredicateName() != mtproto.Predicate_updateDraftMessage {
			continue
		}
		switch upd.GetDraft().GetMessage() {
		case "saved note":
			if upd.GetPeer_PEER().GetUserId() != aSelf.ID {
				t.Fatalf("saved peer %#v", upd.GetPeer_PEER())
			}
			saved = true
		case "user note":
			if upd.GetPeer_PEER().GetUserId() != bSelf.ID {
				t.Fatalf("user peer %#v", upd.GetPeer_PEER())
			}
			user = true
		}
	}
	if !saved || !user {
		t.Fatalf("drafts saved=%v user=%v obj=%T", saved, user, obj)
	}
}
