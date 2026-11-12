package dao

import (
	"context"
	"errors"
	"fmt"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/pkg/historyttl"
)

// stampHistoryTTL copies the dialog's auto-delete timer onto a message that
// is about to be stored. The outbox and inbox processes each call this
// before they persist their own box. The inbox cannot rely on copying
// ttl_period from the message object: that field may be missing after the
// RPC from msg, which would leave the receiver's row at 0.
//
// A lookup that finds no timer leaves a period that was already set, so a
// later inbox lookup that races the dialog write does not clear a stamp
// the outbox already applied.
func (d *Dao) stampHistoryTTL(ctx context.Context, userID int64, peer *mtproto.PeerUtil, message *mtproto.Message) {
	if d == nil || message == nil || peer == nil {
		return
	}
	period := d.historyTTL(ctx, userID, peer)
	if period <= 0 {
		return
	}
	historyttl.Apply(message, period)
}

func (d *Dao) historyTTL(ctx context.Context, userID int64, peer *mtproto.PeerUtil) int32 {
	if d == nil || peer == nil || userID == 0 {
		return 0
	}
	lookup := d.historyTTLQuery
	if lookup == nil {
		if d.DB == nil {
			return 0
		}
		lookup = d.scalarInt32
	}
	return lookupHistoryTTL(ctx, lookup, userID, peer)
}

// historyTTLQuery, when set, replaces scalarInt32 so tests can serve a
// dialogs.ttl_period of 86400 while a stale DialogExt cache still holds 0.
// Production leaves this nil and always reads the column.
func lookupHistoryTTL(ctx context.Context, lookup func(ctx context.Context, query string, args ...interface{}) int32, userID int64, peer *mtproto.PeerUtil) int32 {
	if lookup == nil || peer == nil || userID == 0 {
		return 0
	}
	if peer.PeerType == mtproto.PEER_CHAT {
		if ttl := lookup(ctx, "select ttl_period from chats where id = ? limit 1", peer.PeerId); ttl > 0 {
			return ttl
		}
	}
	if ttl := lookup(ctx, "select ttl_period from dialogs where user_id = ? and peer_id = ? and deleted = 0 and ttl_period > 0 order by ttl_period desc limit 1", userID, peer.PeerId); ttl > 0 {
		return ttl
	}
	if peer.IsUser() {
		other := peer.PeerId
		if other == 0 {
			other = userID
		}
		if ttl := lookup(ctx, "select ttl_period from dialogs where user_id = ? and peer_id = ? and deleted = 0 and ttl_period > 0 order by ttl_period desc limit 1", other, userID); ttl > 0 {
			return ttl
		}
	}
	return 0
}

// ensureDialogTTL copies the chat timer onto a dialog row that was just
// created with the column default of 0. setHistoryTTL updates rows that
// already exist; the service message that follows creates the other side.
func (d *Dao) ensureDialogTTL(ctx context.Context, userID int64, peer *mtproto.PeerUtil) {
	if d == nil || d.DB == nil || peer == nil || userID == 0 {
		return
	}
	ttl := d.historyTTL(ctx, userID, peer)
	if ttl <= 0 {
		return
	}
	_, _ = d.DB.Exec(ctx, "update dialogs set ttl_period = ? where user_id = ? and peer_type = ? and peer_id = ? and ttl_period = 0",
		ttl, userID, peer.PeerType, peer.PeerId)
	// The insert path can leave a cached DialogExt with ttl_period=0.
	d.invalidateDialogCache(ctx, dialog.GetDialogCacheKeyByPeer(userID, peer.PeerType, peer.PeerId))
}

func historyPeriod(message *mtproto.Message) int32 {
	return historyttl.PeriodOf(message)
}

func (d *Dao) scalarInt32(ctx context.Context, query string, args ...interface{}) int32 {
	var row struct {
		V int32 `db:"ttl_period"`
	}
	err := d.DB.QueryRow(ctx, &row, query, args...)
	if err != nil {
		if !errors.Is(err, sqlx.ErrNotFound) {
			return 0
		}
		return 0
	}
	return row.V
}

// RemainingUnread is the dialog unread_count after messages (readInbox, maxID]
// from other people have been read. It is updateReadHistoryInbox.still_unread_count.
func (d *Dao) RemainingUnread(ctx context.Context, userID int64, peerType int32, peerID int64, readInbox, maxID, unread int32) int32 {
	if unread < 0 {
		unread = 0
	}
	if d == nil || d.MessagesDAO == nil || maxID <= readInbox {
		return unread
	}
	did := mtproto.MakeDialogId(userID, peerType, peerID)
	read := d.CommonDAO.CalcSizeByWhere(
		ctx,
		d.MessagesDAO.CalcTableName(userID),
		fmt.Sprintf("user_id = %d AND dialog_id1 = %d AND dialog_id2 = %d AND sender_user_id <> %d AND user_message_box_id > %d AND user_message_box_id <= %d AND deleted = 0",
			userID, did.A, did.B, userID, readInbox, maxID),
	)
	left := unread - int32(read)
	if left < 0 {
		return 0
	}
	return left
}

func (d *Dao) invalidateDialogCache(ctx context.Context, keys ...string) {
	if d == nil || len(keys) == 0 {
		return
	}
	_ = d.CachedConn.DelCache(ctx, keys...)
	dialog.ScheduleDelCache(d.CachedConn, 0, keys...)
}
