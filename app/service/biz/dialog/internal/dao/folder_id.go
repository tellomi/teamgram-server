package dao

import (
	"context"
	"fmt"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"

	"github.com/zeromicro/go-zero/core/logx"
)

type dialogPersistRow struct {
	PeerDialogId int64 `db:"peer_dialog_id"`
	FolderId     int32 `db:"folder_id"`
	TtlPeriod    int32 `db:"ttl_period"`
}

// applyStoredFolderIDs copies dialogs.folder_id and dialogs.ttl_period onto
// cached dialog objects. Redis can still hold a pre-update object:
// setHistoryTTL deletes the row key, but an in-flight getDialogs can write
// the old value back, and the default cache expiry is days long, so a stale
// ttl_period=0 survives a process restart (and a MySQL restart that leaves
// Redis up). Official clients trust these fields: a stale folder_id moves
// the chat back to the main list, and a stale ttl_period=0 hides the
// auto-delete timer even though the column was written.
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
	var rows []dialogPersistRow
	query := fmt.Sprintf(
		"select peer_dialog_id, folder_id, ttl_period from dialogs where user_id = ? and deleted = 0 and peer_dialog_id in (%s)",
		sqlx.InInt64List(ids))
	if err := d.DB.QueryRowsPartial(ctx, &rows, query, userID); err != nil {
		logx.WithContext(ctx).Errorf("dialog.folder_id - user=%d err=%v", userID, err)
		return
	}
	overlayStoredDialogFields(list, rows)
}

// overlayStoredDialogFields is the cache-vs-DB merge used by getDialogs and
// getDialog. Tests in this package call it directly so a stale Redis object
// can be asserted without starting MySQL or Redis.
func overlayStoredDialogFields(list []*dialog.DialogExt, rows []dialogPersistRow) {
	folders := make(map[int64]int32, len(rows))
	ttls := make(map[int64]int32, len(rows))
	for _, row := range rows {
		folders[row.PeerDialogId] = row.FolderId
		ttls[row.PeerDialogId] = row.TtlPeriod
	}
	for _, ext := range list {
		id := storedDialogID(ext)
		if id == 0 || ext == nil || ext.GetDialog() == nil {
			continue
		}
		if folderID, ok := folders[id]; ok {
			ext.Dialog.FolderId = mtproto.MakeFlagsInt32(folderID)
		}
		if period, ok := ttls[id]; ok {
			applyDialogTTL(ext, period)
		}
	}
}

func applyDialogTTL(ext *dialog.DialogExt, period int32) {
	ext.TtlPeriod = period
	if ext.Dialog == nil {
		return
	}
	if period <= 0 {
		ext.Dialog.TtlPeriod = nil
		return
	}
	ext.Dialog.TtlPeriod = mtproto.MakeFlagsInt32(period)
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
