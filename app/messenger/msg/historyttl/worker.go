// Package historyttl deletes cloud messages whose ttl_period has elapsed.
//
// One replica holds Redis key teamgram_history_ttl for the pass. The rows live in MySQL,
// so a restart continues the backlog and a second pass deletes nothing that
// the first pass already removed. Both sides of a private chat, and every
// group member, have their own message box; each box is deleted and that
// user is sent updateDeleteMessages with a pts. Channels are deleted through
// the channel store, which writes updateDeleteChannelMessages.
package historyttl

import (
	"context"
	"errors"
	"os"
	"strconv"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/pkg/historyttl"
	"github.com/zeromicro/go-zero/core/logx"
)

const lockName = "teamgram_history_ttl"

// Start runs the deleter until the process exits. TELLOMI_HISTORY_TTL_INTERVAL
// is the gap between passes in seconds. 0 turns the loop off. The default is
// 2 seconds so a one-second timer used by tests is not left for a minute,
// and an empty pass is a small indexed read.
func Start(d *dao.Dao) {
	if d == nil || d.DB == nil {
		return
	}
	interval := 2 * time.Second
	if v := os.Getenv("TELLOMI_HISTORY_TTL_INTERVAL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			logx.Errorf("history ttl: bad %s=%q", "TELLOMI_HISTORY_TTL_INTERVAL", v)
		} else if n == 0 {
			logx.Info("history ttl: disabled")
			return
		} else {
			interval = time.Duration(n) * time.Second
		}
	}
	go loop(d, interval)
}

func loop(d *dao.Dao, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	run(d)
	for range ticker.C {
		run(d)
	}
}

func run(d *dao.Dao) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	acquired, err := withLock(ctx, d.DB, func() {
		now := int32(time.Now().Unix())
		if n, err := backfillBoxes(ctx, d, 30); err != nil {
			logx.Errorf("history ttl backfill: %v", err)
		} else if n > 0 {
			logx.Infof("history ttl: stamped %d message boxes that were sent after the timer", n)
		}
		if n, err := deleteBoxes(ctx, d, now, 200); err != nil {
			logx.Errorf("history ttl delete: %v", err)
		} else if n > 0 {
			logx.Infof("history ttl: deleted %d message boxes", n)
		}
		if channel.MySQLOpen() {
			if n, err := channel.Shared().ExpireDue(ctx, now, 200); err != nil {
				logx.Errorf("history ttl channel: %v", err)
			} else if n > 0 {
				logx.Infof("history ttl: deleted %d channel messages", n)
			}
		}
	})
	if err != nil {
		logx.Errorf("history ttl: %v", err)
		return
	}
	if !acquired {
		return
	}
}

func withLock(ctx context.Context, db *sqlx.DB, fn func()) (bool, error) {
	var acquired bool
	err := db.Transact(ctx, func(tx *sqlx.Tx) error {
		var row struct {
			Got int64 `db:"got"`
		}
		if err := tx.QueryRow(&row, "select get_lock(?, 0) as got", lockName); err != nil {
			return err
		}
		if row.Got != 1 {
			return nil
		}
		acquired = true
		defer func() { _, _ = tx.Exec("select release_lock(?)", lockName) }()
		fn()
		return nil
	})
	return acquired, err
}

type dueBox struct {
	UserID   int64 `db:"user_id"`
	BoxID    int32 `db:"user_message_box_id"`
	SenderID int64 `db:"sender_user_id"`
	PeerType int32 `db:"peer_type"`
	PeerID   int64 `db:"peer_id"`
	Dialog1  int64 `db:"dialog_id1"`
	Dialog2  int64 `db:"dialog_id2"`
	Date2    int64 `db:"date2"`
	TTL      int32 `db:"ttl_period"`
}

func deleteBoxes(ctx context.Context, d *dao.Dao, now int32, limit int) (int, error) {
	var rows []dueBox
	for _, table := range messageTables(d) {
		var part []dueBox
		q := "select user_id, user_message_box_id, sender_user_id, peer_type, peer_id, dialog_id1, dialog_id2, date2, ttl_period from " + table + " where deleted = 0 and ttl_period > 0 and date2 > 0 and date2 + ttl_period <= ? order by date2 limit ?"
		if err := d.DB.QueryRows(ctx, &part, q, now, limit); err != nil {
			return 0, err
		}
		rows = append(rows, part...)
		if len(rows) >= limit {
			rows = rows[:limit]
			break
		}
	}
	boxes := make([]historyttl.Box, 0, len(rows))
	byKey := map[[2]int64]dueBox{}
	for _, row := range rows {
		box := historyttl.Box{
			UserID: row.UserID, BoxID: row.BoxID, SenderID: row.SenderID,
			PeerType: row.PeerType, PeerID: row.PeerID, Date: int32(row.Date2), TTL: row.TTL,
		}
		boxes = append(boxes, box)
		byKey[[2]int64{row.UserID, int64(row.BoxID)}] = row
	}
	due := historyttl.Due(boxes, now)
	if len(due) == 0 {
		return 0, nil
	}
	byUser := map[int64][]historyttl.Box{}
	for _, box := range due {
		byUser[box.UserID] = append(byUser[box.UserID], box)
	}
	var deleted int
	for userID, list := range byUser {
		n, err := deleteUser(ctx, d, userID, list, byKey)
		if err != nil {
			return deleted, err
		}
		deleted += n
	}
	return deleted, nil
}

func deleteUser(ctx context.Context, d *dao.Dao, userID int64, list []historyttl.Box, raw map[[2]int64]dueBox) (int, error) {
	ids := make([]int32, 0, len(list))
	for _, box := range list {
		ids = append(ids, box.BoxID)
	}
	// Same statement messages.deleteMessages uses: deleted = 1, and only rows
	// that are still live. Media retention treats deleted = 0 as the reference,
	// so this drops the message's media reference the way a revoke does.
	affected, err := d.MessagesDAO.DeleteMessagesByMessageIdList(ctx, userID, ids)
	if err != nil {
		return 0, err
	}
	if affected == 0 {
		return 0, nil
	}
	byDialog := map[[2]int64][]historyttl.Box{}
	for _, box := range list {
		byDialog[[2]int64{int64(box.PeerType), box.PeerID}] = append(byDialog[[2]int64{int64(box.PeerType), box.PeerID}], box)
	}
	for _, boxes := range byDialog {
		row := raw[[2]int64{boxes[0].UserID, int64(boxes[0].BoxID)}]
		if err := fixDialog(ctx, d, userID, boxes, row.Dialog1, row.Dialog2); err != nil {
			logx.WithContext(ctx).Errorf("history ttl dialog %d: %v", userID, err)
		}
	}
	pts := d.IDGenClient2.NextNPtsId(ctx, userID, len(ids))
	upd := mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{
		Messages:  ids,
		Pts_INT32: pts,
		PtsCount:  int32(len(ids)),
	}).To_Update()
	d.AddToPtsQueue(ctx, userID, pts, int32(len(ids)), upd)
	if d.SyncClient != nil {
		_, _ = d.SyncClient.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{
			UserId:  userID,
			Updates: mtproto.MakeUpdatesByUpdates(upd),
		})
	}
	return int(affected), nil
}

func fixDialog(ctx context.Context, d *dao.Dao, userID int64, boxes []historyttl.Box, dialog1, dialog2 int64) error {
	peerType, peerID := boxes[0].PeerType, boxes[0].PeerID
	headDO, err := d.DialogsDAO.SelectDialog(ctx, userID, peerType, peerID)
	if err != nil || headDO == nil {
		return err
	}
	head := historyttl.Head{
		ReadInbox: headDO.ReadInboxMaxId,
		Unread:    headDO.UnreadCount,
		Top:       headDO.TopMessage,
		Date:      headDO.Date2,
	}
	unread := historyttl.UnreadAfter(head, userID, boxes)
	lastID, lastDate, err := lastLiveMessage(ctx, d, userID, dialog1, dialog2)
	if err != nil {
		return err
	}
	_, _, err = d.CachedConn.Exec(ctx, func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
		_, err2 := d.DialogsDAO.UpdateCustomMap(ctx, map[string]interface{}{
			"top_message":  lastID,
			"unread_count": unread,
			"date2":        lastDate,
		}, userID, peerType, peerID)
		return 0, 0, err2
	}, dialog.GetDialogCacheKeyByPeer(userID, peerType, peerID))
	return err
}

func lastLiveMessage(ctx context.Context, d *dao.Dao, userID, dialog1, dialog2 int64) (int32, int64, error) {
	var row struct {
		ID   int32 `db:"user_message_box_id"`
		Date int64 `db:"date2"`
	}
	table := d.MessagesDAO.CalcTableName(userID)
	err := d.DB.QueryRow(ctx, &row, "select user_message_box_id, date2 from "+table+" where user_id = ? and dialog_id1 = ? and dialog_id2 = ? and deleted = 0 order by user_message_box_id desc limit 1", userID, dialog1, dialog2)
	if errors.Is(err, sqlx.ErrNotFound) {
		return 0, time.Now().Unix(), nil
	}
	if err != nil {
		return 0, 0, err
	}
	return row.ID, row.Date, nil
}

func messageTables(d *dao.Dao) []string {
	n := 0
	if d.MessagesDAO != nil {
		n = d.MessagesDAO.ShardingSize
	}
	if n <= 1 {
		return []string{"messages"}
	}
	out := make([]string, n)
	for i := 0; i < n; i++ {
		out[i] = "messages_" + strconv.Itoa(i)
	}
	return out
}
