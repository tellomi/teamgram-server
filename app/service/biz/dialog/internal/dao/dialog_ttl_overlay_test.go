package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// TestOverlayStoredDialogTTLFromDB reproduces the getDialogs failure seen on
// PR #309's reliability job (ttl_period 0 after setHistoryTTL, before the
// process restart): Redis still holds the pre-timer dialog object, while
// dialogs.ttl_period is already 86400. getDialogs must serve the column, not
// the cached zero. The same overlay turns the timer off when the column is 0.
func TestOverlayStoredDialogTTLFromDB(t *testing.T) {
	peerID := int64(20)
	dialogID := mtproto.MakePeerDialogId(mtproto.PEER_USER, peerID)
	cached := dialog.MakeTLDialogExt(&dialog.DialogExt{
		TtlPeriod: 0,
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Peer:      mtproto.MakePeer(mtproto.PEER_USER, peerID),
			FolderId:  mtproto.MakeFlagsInt32(0),
			TtlPeriod: nil,
		}).To_Dialog(),
	}).To_DialogExt()

	overlayStoredDialogFields([]*dialog.DialogExt{cached}, []dialogPersistRow{{
		PeerDialogId: dialogID,
		FolderId:     1,
		TtlPeriod:    86400,
	}})

	if cached.GetTtlPeriod() != 86400 {
		t.Fatalf("ext ttl_period %d, want 86400", cached.GetTtlPeriod())
	}
	flag := cached.GetDialog().GetTtlPeriod()
	if flag == nil || flag.GetValue() != 86400 {
		t.Fatalf("dialog ttl flag %#v, want 86400", flag)
	}
	folder := cached.GetDialog().GetFolderId()
	if folder == nil || folder.GetValue() != 1 {
		t.Fatalf("folder_id %#v, want 1 (existing overlay must still apply)", folder)
	}

	overlayStoredDialogFields([]*dialog.DialogExt{cached}, []dialogPersistRow{{
		PeerDialogId: dialogID,
		FolderId:     1,
		TtlPeriod:    0,
	}})
	if cached.GetTtlPeriod() != 0 {
		t.Fatalf("ext ttl_period %d after timer off, want 0", cached.GetTtlPeriod())
	}
	if cached.GetDialog().GetTtlPeriod() != nil {
		t.Fatalf("dialog ttl flag %#v after timer off, want nil", cached.GetDialog().GetTtlPeriod())
	}
}

func TestOverlayStoredDialogTTLLeavesUnknownRowsAlone(t *testing.T) {
	cached := dialog.MakeTLDialogExt(&dialog.DialogExt{
		TtlPeriod: 0,
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Peer: mtproto.MakePeer(mtproto.PEER_USER, 99),
		}).To_Dialog(),
	}).To_DialogExt()
	overlayStoredDialogFields([]*dialog.DialogExt{cached}, []dialogPersistRow{{
		PeerDialogId: mtproto.MakePeerDialogId(mtproto.PEER_USER, 20),
		TtlPeriod:    86400,
	}})
	if cached.GetTtlPeriod() != 0 {
		t.Fatalf("unrelated row overwrote ttl_period to %d", cached.GetTtlPeriod())
	}
	if cached.GetDialog().GetTtlPeriod() != nil {
		t.Fatalf("unrelated row set ttl flag %#v", cached.GetDialog().GetTtlPeriod())
	}
}
