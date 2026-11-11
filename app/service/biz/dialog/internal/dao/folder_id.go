package dao

import (
	"context"
	"fmt"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"github.com/zeromicro/go-zero/core/logx"
)

type folderRow struct {
	PeerDialogId int64 `db:"peer_dialog_id"`
	FolderId     int32 `db:"folder_id"`
	TtlPeriod    int32 `db:"ttl_period"`
}

// applyStoredFolderIDs copies dialogs.folder_id and dialogs.ttl_period onto
// cached dialog objects. Redis can still hold the object from before a
// folder move or messages.setHistoryTTL. The official client trusts those
// fields: a stale folder_id moves the chat back to the main list, and a
// stale ttl_period makes the auto-delete timer look unset.
func (d *Dao) applyStoredFolderIDs(ctx context.Context, userID int64, list []*dialog.DialogExt) {
	if d == nil || d.Mysql == nil || d.DB == nil || userID == 0 || len(list) == 0 {
		return
	}
	ids := make([]int64, 0, len(list))
	for _, ext := range list {
		if id := storedDialogID(ext); id != 0 {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return
	}
	var rows []folderRow
	query := fmt.Sprintf(
		"select peer_dialog_id, folder_id, ttl_period from dialogs where user_id = ? and deleted = 0 and peer_dialog_id in (%s)",
		sqlx.InInt64List(ids))
	if err := d.DB.QueryRowsPartial(ctx, &rows, query, userID); err != nil {
		logx.WithContext(ctx).Errorf("dialog.folder_id - user=%d err=%v", userID, err)
		return
	}
	stored := make(map[int64]folderRow, len(rows))
	for _, row := range rows {
		stored[row.PeerDialogId] = row
	}
	for _, ext := range list {
		id := storedDialogID(ext)
		row, ok := stored[id]
		if !ok || ext == nil || ext.GetDialog() == nil {
			continue
		}
		ext.Dialog.FolderId = mtproto.MakeFlagsInt32(row.FolderId)
		applyStoredTTL(ext, row.TtlPeriod)
	}
}

// applyStoredTTL writes the dialog row's timer onto a cached object.
// Zero and negative values clear the flag. MakeFlagsInt32 already drops
// zero; the <= 0 check also drops a negative column.
func applyStoredTTL(ext *dialog.DialogExt, period int32) {
	if ext == nil || ext.GetDialog() == nil {
		return
	}
	ext.TtlPeriod = period
	ttl := mtproto.MakeFlagsInt32(period)
	if period <= 0 {
		ttl = nil
	}
	ext.Dialog.TtlPeriod = ttl
}

func storedDialogID(ext *dialog.DialogExt) int64 {
	if ext == nil || ext.GetDialog() == nil || ext.GetDialog().GetPeer() == nil {
		return 0
	}
	peer := mtproto.FromPeer(ext.GetDialog().GetPeer())
	if peer == nil || peer.PeerId == 0 {
		return 0
	}
	return mtproto.MakePeerDialogId(peer.PeerType, peer.PeerId)
}
