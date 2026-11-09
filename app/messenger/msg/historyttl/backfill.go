package historyttl

import (
	"context"
	"sync"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dao"
	ttl "github.com/teamgram/teamgram-server/pkg/historyttl"
)

// Backfill walks dialogs and basic groups that already have a timer and
// stamps ttl_period onto messages sent after the set-timer service message.
// Messages sent before that notice stay. A second pass updates nothing
// because it only writes rows whose ttl_period is still 0.
//
// The cursor is in memory. A restart starts again; the updates are the same.
var backfill struct {
	sync.Mutex
	cursor    int64
	dialogs   bool
	chats     bool
	chatAfter int64
}

func backfillBoxes(ctx context.Context, d *dao.Dao, limit int) (int, error) {
	backfill.Lock()
	defer backfill.Unlock()
	if backfill.dialogs && backfill.chats {
		return 0, nil
	}
	if !backfill.dialogs {
		return backfillDialogPage(ctx, d, limit)
	}
	return backfillChatPage(ctx, d, limit)
}

type ttlSeed struct {
	ID       int64 `db:"id"`
	UserID   int64 `db:"user_id"`
	PeerType int32 `db:"peer_type"`
	PeerID   int64 `db:"peer_id"`
}

func backfillDialogPage(ctx context.Context, d *dao.Dao, limit int) (int, error) {
	var seeds []ttlSeed
	err := d.DB.QueryRows(ctx, &seeds, "select id, user_id, peer_type, peer_id from dialogs where deleted = 0 and ttl_period > 0 and id > ? order by id limit ?", backfill.cursor, limit)
	if err != nil {
		return 0, err
	}
	if len(seeds) == 0 {
		backfill.dialogs = true
		return 0, nil
	}
	var n int
	for _, seed := range seeds {
		stamped, err := stampScope(ctx, d, seed.PeerType, seed.PeerID, seed.UserID)
		if err != nil {
			return n, err
		}
		n += stamped
		backfill.cursor = seed.ID
	}
	return n, nil
}

type chatSeed struct {
	ID int64 `db:"id"`
}

func backfillChatPage(ctx context.Context, d *dao.Dao, limit int) (int, error) {
	var seeds []chatSeed
	err := d.DB.QueryRows(ctx, &seeds, "select id from chats where ttl_period > 0 and id > ? order by id limit ?", backfill.chatAfter, limit)
	if err != nil {
		// A database without the chats table is not a group deployment.
		backfill.chats = true
		return 0, nil
	}
	if len(seeds) == 0 {
		backfill.chats = true
		return 0, nil
	}
	var n int
	for _, seed := range seeds {
		stamped, err := stampScope(ctx, d, mtproto.PEER_CHAT, seed.ID, 0)
		if err != nil {
			return n, err
		}
		n += stamped
		backfill.chatAfter = seed.ID
	}
	return n, nil
}

type ownerRow struct {
	UserID int64 `db:"user_id"`
}

func stampScope(ctx context.Context, d *dao.Dao, peerType int32, peerID, hint int64) (int, error) {
	owners := map[int64]struct{}{}
	if hint > 0 {
		owners[hint] = struct{}{}
	}
	if peerType == mtproto.PEER_USER && peerID > 0 {
		owners[peerID] = struct{}{}
	}
	var rows []ownerRow
	_ = d.DB.QueryRows(ctx, &rows, "select distinct user_id from dialogs where peer_type = ? and peer_id = ? and deleted = 0", peerType, peerID)
	for _, row := range rows {
		if row.UserID > 0 {
			owners[row.UserID] = struct{}{}
		}
	}
	if peerType == mtproto.PEER_CHAT {
		var parts []ownerRow
		if err := d.DB.QueryRows(ctx, &parts, "select user_id from chat_participants where chat_id = ? and state = 0", peerID); err == nil {
			for _, row := range parts {
				if row.UserID > 0 {
					owners[row.UserID] = struct{}{}
				}
			}
		}
	}
	var actions []ttl.Action
	for userID := range owners {
		found, err := loadActions(ctx, d, userID, peerType, peerID)
		if err != nil {
			return 0, err
		}
		actions = append(actions, found...)
	}
	if len(actions) == 0 {
		return 0, nil
	}
	var n int
	for userID := range owners {
		// Private chats store the other user as peer_id. Group rows all use the chat id.
		peer := peerID
		ptype := peerType
		if peerType == mtproto.PEER_USER && userID == peerID && hint > 0 {
			peer = hint
		}
		stamped, err := stampUser(ctx, d, userID, ptype, peer, actions)
		if err != nil {
			return n, err
		}
		n += stamped
	}
	return n, nil
}

type actionRow struct {
	Date int64  `db:"date2"`
	Data string `db:"message_data"`
}

func loadActions(ctx context.Context, d *dao.Dao, userID int64, peerType int32, peerID int64) ([]ttl.Action, error) {
	var rows []actionRow
	table := d.MessagesDAO.CalcTableName(userID)
	err := d.DB.QueryRows(ctx, &rows, "select date2, message_data from "+table+" where user_id = ? and peer_type = ? and peer_id = ? and deleted = 0 and message_data like '%messageActionSetMessagesTTL%'", userID, peerType, peerID)
	if err != nil {
		return nil, err
	}
	var out []ttl.Action
	for _, row := range rows {
		action, ok := ttl.ParseAction(row.Data)
		if !ok {
			continue
		}
		if action.Date == 0 {
			action.Date = int32(row.Date)
		}
		out = append(out, action)
	}
	return out, nil
}

type plainRow struct {
	ID   int32 `db:"user_message_box_id"`
	Date int64 `db:"date2"`
}

func stampUser(ctx context.Context, d *dao.Dao, userID int64, peerType int32, peerID int64, actions []ttl.Action) (int, error) {
	table := d.MessagesDAO.CalcTableName(userID)
	var n int
	var after int32
	for {
		var rows []plainRow
		err := d.DB.QueryRows(ctx, &rows, "select user_message_box_id, date2 from "+table+" where user_id = ? and peer_type = ? and peer_id = ? and deleted = 0 and ttl_period = 0 and user_message_box_id > ? order by user_message_box_id limit 500", userID, peerType, peerID, after)
		if err != nil {
			return n, err
		}
		if len(rows) == 0 {
			return n, nil
		}
		after = rows[len(rows)-1].ID
		byPeriod := map[int32][]int32{}
		for _, row := range rows {
			period := ttl.PeriodAt(actions, int32(row.Date))
			if period <= 0 {
				continue
			}
			byPeriod[period] = append(byPeriod[period], row.ID)
		}
		for period, ids := range byPeriod {
			q := "update " + table + " set ttl_period = ? where user_id = ? and deleted = 0 and ttl_period = 0 and user_message_box_id in (" + sqlx.InInt32List(ids) + ")"
			res, err := d.DB.Exec(ctx, q, period, userID)
			if err != nil {
				return n, err
			}
			affected, _ := res.RowsAffected()
			n += int(affected)
		}
		if len(rows) < 500 {
			return n, nil
		}
	}
}
