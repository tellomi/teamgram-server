package dao

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
)

// ReplayKind is what a later send with the same random ids should do.
// The sender row commits before the inbox publish. A retry must be able to
// tell a row that was never written from one that is waiting to be delivered.
type ReplayKind int

const (
	// ReplayMissing means at least one random id has no row. The caller inserts.
	ReplayMissing ReplayKind = iota
	// ReplayLive means every random id is present and at least one row is
	// still visible. The caller publishes the inbox again and does not insert.
	ReplayLive
	// ReplayDeleted means every random id is present and every row is deleted.
	// The caller does not insert and does not publish.
	ReplayDeleted
)

// ClassifyOutboxReplay loads the sender's rows for these client random ids.
// live is the subset that is still visible and is what a retry may deliver.
// all includes a deleted row so the RPC result can still name that id.
// A random id of 0 is not an idempotency key; the call is ReplayMissing.
func (d *Dao) ClassifyOutboxReplay(ctx context.Context, fromUserID int64, randomIDs []int64) (kind ReplayKind, live, all []*mtproto.MessageBox, err error) {
	if d == nil || d.MessagesDAO == nil || fromUserID == 0 || len(randomIDs) == 0 {
		return ReplayMissing, nil, nil, nil
	}
	for _, id := range randomIDs {
		if id == 0 {
			return ReplayMissing, nil, nil, nil
		}
	}
	anyLive := false
	for _, id := range randomIDs {
		row, err := d.messageByRandom(ctx, fromUserID, fromUserID, id)
		if err != nil {
			return ReplayMissing, nil, nil, err
		}
		if row == nil {
			return ReplayMissing, nil, nil, nil
		}
		box := makeMessageBoxByDO(row)
		if err = d.attachStoredPts(ctx, fromUserID, box); err != nil {
			return ReplayMissing, nil, nil, err
		}
		all = append(all, box)
		if row.Deleted {
			continue
		}
		anyLive = true
		live = append(live, box)
	}
	if !anyLive {
		return ReplayDeleted, nil, all, nil
	}
	return ReplayLive, live, all, nil
}

func (d *Dao) messageByRandom(ctx context.Context, ownerUserID, senderUserID, randomID int64) (*dataobject.MessagesDO, error) {
	if d == nil || d.DB == nil || d.MessagesDAO == nil || ownerUserID == 0 || randomID == 0 {
		return nil, nil
	}
	query := fmt.Sprintf(`select user_id, user_message_box_id, dialog_id1, dialog_id2, dialog_message_id, sender_user_id, peer_type, peer_id, random_id, message_filter_type, message_data, message, mentioned, media_unread, date2, ttl_period, deleted from %s where user_id = ? and sender_user_id = ? and random_id = ? order by deleted asc limit 1`, d.MessagesDAO.CalcTableName(ownerUserID))
	do := &dataobject.MessagesDO{}
	err := d.DB.QueryRowPartial(ctx, do, query, ownerUserID, senderUserID, randomID)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if do.UserMessageBoxId == 0 && do.RandomId == 0 {
		return nil, nil
	}
	return do, nil
}

func (d *Dao) attachStoredPts(ctx context.Context, userID int64, box *mtproto.MessageBox) error {
	if box == nil || box.MessageId == 0 {
		return nil
	}
	pts, count, ok, err := d.FindNewMessagePts(ctx, userID, box.MessageId)
	if err != nil {
		return err
	}
	fallback := d.IDGenClient2.CurrentPtsId(ctx, userID)
	box.Pts, box.PtsCount = chooseReplayPts(pts, count, ok, fallback)
	return nil
}

// chooseReplayPts keeps the pts that was stored with the message.
// CurrentPtsId is only a fallback when that row was never written. Using the
// current counter as the update pts would make another device apply this
// message as a newer update.
func chooseReplayPts(storedPts, storedCount int32, storedOK bool, currentPts int32) (int32, int32) {
	if storedOK && storedPts > 0 {
		if storedCount <= 0 {
			storedCount = 1
		}
		return storedPts, storedCount
	}
	if currentPts > 0 {
		return currentPts, 1
	}
	return 0, 0
}

// FindNewMessagePts returns the pts row already stored for this box id.
func (d *Dao) FindNewMessagePts(ctx context.Context, userID int64, boxID int32) (pts, count int32, ok bool, err error) {
	if d == nil || d.DB == nil || userID == 0 || boxID == 0 {
		return 0, 0, false, nil
	}
	query := `select pts, pts_count, update_data from user_pts_updates where user_id = ? and update_type = ? and update_data like ? order by pts asc limit 20`
	var rows []dataobject.UserPtsUpdatesDO
	err = d.DB.QueryRowsPartial(ctx, &rows, query, userID, mtproto.PTS_UPDATE_NEW_MESSAGE, "%"+ptsBoxNeedle(boxID)+"%")
	if err != nil {
		return 0, 0, false, err
	}
	for i := range rows {
		if !updateCarriesBox(rows[i].UpdateData, boxID) {
			continue
		}
		count = rows[i].PtsCount
		if count <= 0 {
			count = 1
		}
		return rows[i].Pts, count, true, nil
	}
	return 0, 0, false, nil
}

func ptsBoxNeedle(boxID int32) string {
	return `"id":` + strconv.FormatInt(int64(boxID), 10)
}

// updateCarriesBox reports whether this stored updateNewMessage is for boxID.
// A prefix of the id, such as 12 inside 123, does not match.
func updateCarriesBox(updateData string, boxID int32) bool {
	if updateData == "" || boxID == 0 {
		return false
	}
	var doc struct {
		Message struct {
			ID int32 `json:"id"`
		} `json:"message_MESSAGE"`
	}
	if err := json.Unmarshal([]byte(updateData), &doc); err != nil {
		return false
	}
	return doc.Message.ID == boxID
}
