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
}

// applyStoredFolderIDs copies dialogs.folder_id onto cached dialog objects.
// Redis can still hold the pre-archive object; the official client trusts
// dialog.folder_id and would move the chat back to the main list.
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
		"select peer_dialog_id, folder_id from dialogs where user_id = ? and deleted = 0 and peer_dialog_id in (%s)",
		sqlx.InInt64List(ids))
	if err := d.DB.QueryRowsPartial(ctx, &rows, query, userID); err != nil {
		logx.WithContext(ctx).Errorf("dialog.folder_id - user=%d err=%v", userID, err)
		return
	}
	folders := make(map[int64]int32, len(rows))
	for _, row := range rows {
		folders[row.PeerDialogId] = row.FolderId
	}
	for _, ext := range list {
		id := storedDialogID(ext)
		folderID, ok := folders[id]
		if !ok || ext == nil || ext.GetDialog() == nil {
			continue
		}
		ext.Dialog.FolderId = mtproto.MakeFlagsInt32(folderID)
	}
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
