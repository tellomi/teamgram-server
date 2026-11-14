package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

func TestDraftCacheKeysCoverDialogAndDraftList(t *testing.T) {
	keys := draftCacheKeys(7, mtproto.PEER_USER, 8)
	wantDialog := dialog.GetDialogCacheKeyByPeer(7, mtproto.PEER_USER, 8)
	wantList := dialog.GetAllDraftIdListCacheKey(7)
	var dialogHit, listHit bool
	for _, k := range keys {
		if k == wantDialog {
			dialogHit = true
		}
		if k == wantList {
			listHit = true
		}
		if k == "" {
			t.Fatal("empty cache key")
		}
	}
	if !dialogHit || !listHit {
		t.Fatalf("keys %v", keys)
	}
}
