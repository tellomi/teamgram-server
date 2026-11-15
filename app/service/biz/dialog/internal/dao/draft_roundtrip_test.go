package dao

import (
	"testing"

	"github.com/teamgram/marmota/pkg/hack"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"
	"github.com/zeromicro/go-zero/core/jsonx"
)

func TestMakeDialogRestoresDraftText(t *testing.T) {
	msg := mtproto.MakeTLDraftMessage(&mtproto.DraftMessage{
		Message:    "draft-from-a",
		Date_INT32: 100,
	}).To_DraftMessage()
	raw, err := jsonx.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	ext := (&Dao{}).MakeDialog(&dataobject.DialogsDO{
		PeerType:         mtproto.PEER_USER,
		PeerId:           8,
		DraftType:        2,
		DraftMessageData: hack.String(raw),
	})
	if got := ext.GetDialog().GetDraft().GetMessage(); got != "draft-from-a" {
		t.Fatalf("draft message %q", got)
	}
	if ext.GetDialog().GetDraft().GetPredicateName() != mtproto.Predicate_draftMessage {
		t.Fatalf("predicate %s", ext.GetDialog().GetDraft().GetPredicateName())
	}
}
