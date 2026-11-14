// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package core

import (
	"context"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// historyTTLStore is the dialog row writer. *mysql_dao.DialogsDAO implements it.
// Tests pass a fake so both sides of a private chat can be asserted without MySQL.
type historyTTLStore interface {
	UpdateCustomMapTx(tx *sqlx.Tx, cMap map[string]interface{}, userId int64, peerType int32, peerId int64) (int64, error)
	// InsertDialogTTLTx creates a dialog that did not exist yet and stores the
	// timer on it. An existing row keeps its other columns and takes this timer.
	InsertDialogTTLTx(tx *sqlx.Tx, userID int64, peerType int32, peerID int64, ttl int32) error
}

// DialogSetHistoryTTL
// dialog.setHistoryTTL user_id:long peer_type:int peer_id:long ttl_period:int = Bool;
func (c *DialogCore) DialogSetHistoryTTL(in *dialog.TLDialogSetHistoryTTL) (*mtproto.Bool, error) {
	// Drop the cached dialog. getDialogs and getFullUser otherwise keep the
	// timer from before this write until the cache entry expires.
	// ExecInvalidate deletes those keys only when the callback returns nil,
	// so a rolled-back write leaves the previous cache entry in place, and
	// then deletes them again after a short delay so an in-flight getDialogs
	// cannot SetEx the pre-write object.
	keys := historyTTLCacheKeys(in.UserId, in.PeerType, in.PeerId)
	if in.PeerType == mtproto.PEER_CHAT {
		var members []struct {
			UserID int64 `db:"user_id"`
		}
		_ = c.svcCtx.Dao.DB.QueryRows(c.ctx, &members, "select user_id from dialogs where peer_type = ? and peer_id = ? and deleted = 0", in.PeerType, in.PeerId)
		for _, member := range members {
			keys = append(keys, dialog.GetDialogCacheKeyByPeer(member.UserID, in.PeerType, in.PeerId))
		}
	}
	_, _, err := c.svcCtx.Dao.ExecInvalidate(
		c.ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			if err := writeHistoryTTL(ctx, conn, c.svcCtx.Dao.DialogsDAO, in); err != nil {
				return 0, 0, err
			}
			return 0, 1, nil
		},
		keys...)
	if err != nil {
		c.Logger.Errorf("dialog.setHistoryTTL - error: %v", err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func historyTTLCacheKeys(userID int64, peerType int32, peerID int64) []string {
	keys := []string{dialog.GetDialogCacheKeyByPeer(userID, peerType, peerID)}
	if peerType == mtproto.PEER_USER && peerID != userID {
		keys = append(keys, dialog.GetDialogCacheKeyByPeer(peerID, peerType, userID))
	}
	return keys
}

// writeHistoryTTL stores the timer on both sides of a private chat in one
// transaction. TxWrapper rolls the first update back when applyHistoryTTL
// returns an error, because it returns that error from the transaction body.
func writeHistoryTTL(ctx context.Context, conn *sqlx.DB, store historyTTLStore, in *dialog.TLDialogSetHistoryTTL) error {
	result := sqlx.TxWrapper(ctx, conn, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
		if err := applyHistoryTTL(tx, store, in); err != nil {
			result.Err = err
			return
		}
		// One timer for a basic group. Every member dialog and the chat row
		// are updated in this same transaction, so a later send from any
		// member stamps the same ttl_period.
		if in.PeerType != mtproto.PEER_CHAT {
			return
		}
		if _, err := tx.Exec("update dialogs set ttl_period = ? where peer_type = ? and peer_id = ?", in.TtlPeriod, in.PeerType, in.PeerId); err != nil {
			result.Err = err
			return
		}
		if _, err := tx.Exec("update chats set ttl_period = ? where id = ?", in.TtlPeriod, in.PeerId); err != nil {
			result.Err = err
			return
		}
		// Members who have never opened the group have no dialogs row. The
		// update above does not create one. Insert those rows so getPeerDialogs
		// reads the same timer the chat row now stores.
		ids, err := normalChatMemberIDs(tx, in.PeerId)
		if err != nil {
			result.Err = err
			return
		}
		if err := upsertChatMemberTTLs(tx, store, ids, in.PeerId, in.TtlPeriod); err != nil {
			result.Err = err
		}
	})
	return result.Err
}

func applyHistoryTTL(tx *sqlx.Tx, store historyTTLStore, in *dialog.TLDialogSetHistoryTTL) error {
	if err := writeOneHistoryTTL(tx, store, in.UserId, in.PeerType, in.PeerId, in.TtlPeriod); err != nil {
		return err
	}
	// The other side of a private chat stores the same timer. A chat id is
	// not a user id, so the reverse row is only written for users. Saved
	// messages are one row.
	if in.PeerType == mtproto.PEER_USER && in.PeerId != in.UserId {
		return writeOneHistoryTTL(tx, store, in.PeerId, in.PeerType, in.UserId, in.TtlPeriod)
	}
	return nil
}

// writeOneHistoryTTL updates an existing dialog. A missing row is inserted
// with this timer. Official clients allow messages.setHistoryTTL before the
// first message, and getPeerDialogs / users.getFullUser read that column.
func writeOneHistoryTTL(tx *sqlx.Tx, store historyTTLStore, userID int64, peerType int32, peerID int64, ttl int32) error {
	n, err := store.UpdateCustomMapTx(tx, map[string]interface{}{"ttl_period": ttl}, userID, peerType, peerID)
	if err != nil || n > 0 {
		return err
	}
	return store.InsertDialogTTLTx(tx, userID, peerType, peerID, ttl)
}

// upsertChatMemberTTLs stores the group timer on every current member's
// dialog, creating the row when that member has not opened the group.
func upsertChatMemberTTLs(tx *sqlx.Tx, store historyTTLStore, memberIDs []int64, chatID int64, ttl int32) error {
	seen := map[int64]bool{}
	for _, id := range memberIDs {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		if err := writeOneHistoryTTL(tx, store, id, mtproto.PEER_CHAT, chatID, ttl); err != nil {
			return err
		}
	}
	return nil
}

func normalChatMemberIDs(tx *sqlx.Tx, chatID int64) ([]int64, error) {
	if tx == nil || chatID == 0 {
		return nil, nil
	}
	var rows []struct {
		UserID int64 `db:"user_id"`
	}
	if err := tx.QueryRowsPartial(&rows, "select user_id from chat_participants where chat_id = ? and state = 0", chatID); err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserID)
	}
	return ids, nil
}
