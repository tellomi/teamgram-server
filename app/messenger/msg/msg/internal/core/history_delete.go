package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/jsonx"
)

// deleteHistoryBatch is how many messages one messages.deleteHistory call
// removes. Telegram clients call the method again while offset > 0.
const deleteHistoryBatch = 100

// historyRow is one live message box that a deleteHistory call may remove.
type historyRow struct {
	BoxID  int32
	DataID int64
	Date   int32
}

// historyPlan is one slice of messages.deleteHistory.
// Offset is how many matching messages remain after this slice, so the
// client repeats the same request until it is 0.
type historyPlan struct {
	Delete  []historyRow
	Rewrite *historyRow
	Offset  int32
	Read    bool
}

// planHistoryDelete chooses the next slice.
// rows are newest first and may be shorter than total.
// protectNewest keeps the newest match and rewrites it only on the last slice.
// That is just_clear for a full clear: the dialog stays, with a history-clear
// service message, and the rest of the history goes away in batches.
func planHistoryDelete(total int32, rows []historyRow, batch int, protectNewest bool) historyPlan {
	if batch < 1 {
		batch = 1
	}
	if total < 0 {
		total = 0
	}
	if protectNewest {
		if total == 0 || len(rows) == 0 {
			return historyPlan{}
		}
		head := rows[0]
		deletable := total - 1
		if deletable < 0 {
			deletable = 0
		}
		var rest []historyRow
		if len(rows) > 1 {
			rest = rows[1:]
		}
		n := deletable
		if int32(batch) < n {
			n = int32(batch)
		}
		if int32(len(rest)) < n {
			n = int32(len(rest))
		}
		del := append([]historyRow(nil), rest[:n]...)
		offset := deletable - n
		if offset < 0 {
			offset = 0
		}
		p := historyPlan{Delete: del, Offset: offset, Read: offset == 0}
		if offset == 0 {
			p.Rewrite = &head
		}
		return p
	}
	n := total
	if int32(batch) < n {
		n = int32(batch)
	}
	if int32(len(rows)) < n {
		n = int32(len(rows))
	}
	if n < 0 {
		n = 0
	}
	del := append([]historyRow(nil), rows[:n]...)
	offset := total - n
	if offset < 0 {
		offset = 0
	}
	return historyPlan{Delete: del, Offset: offset, Read: offset == 0 && n > 0}
}

// historyUpdates builds the pts updates for one slice.
// pts is the pts value after this slice. The sum of pts_count equals the
// number of pts allocated, and the last update ends on pts.
func historyUpdates(peer *mtproto.PeerUtil, plan historyPlan, clear *mtproto.Message, pts int32) ([]*mtproto.Update, int32) {
	n := int32(len(plan.Delete))
	extra := int32(0)
	if plan.Read {
		extra++
	}
	if plan.Rewrite != nil && clear != nil {
		extra++
	}
	total := n + extra
	if total == 0 {
		return nil, 0
	}
	var ups []*mtproto.Update
	used := int32(0)
	if n > 0 {
		ids := make([]int32, n)
		for i, row := range plan.Delete {
			ids[i] = row.BoxID
		}
		ups = append(ups, mtproto.MakeTLUpdateDeleteMessages(&mtproto.Update{
			Messages:  ids,
			Pts_INT32: pts - extra,
			PtsCount:  n,
		}).To_Update())
		used += n
	}
	if plan.Read {
		maxID := int32(0)
		for _, row := range plan.Delete {
			if row.BoxID > maxID {
				maxID = row.BoxID
			}
		}
		if plan.Rewrite != nil && plan.Rewrite.BoxID > maxID {
			maxID = plan.Rewrite.BoxID
		}
		readPts := pts
		if plan.Rewrite != nil && clear != nil {
			readPts = pts - 1
		}
		ups = append(ups, mtproto.MakeTLUpdateReadHistoryInbox(&mtproto.Update{
			Peer_PEER: peer.ToPeer(),
			MaxId:     maxID,
			Pts_INT32: readPts,
			PtsCount:  1,
		}).To_Update())
		used++
	}
	if plan.Rewrite != nil && clear != nil {
		ups = append(ups, mtproto.MakeTLUpdateEditMessage(&mtproto.Update{
			Message_MESSAGE: clear,
			Pts_INT32:       pts,
			PtsCount:        1,
		}).To_Update())
		used++
	}
	return ups, used
}

func historyClearMessage(userID int64, peer *mtproto.PeerUtil, row historyRow) *mtproto.Message {
	return mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		Id:     row.BoxID,
		FromId: mtproto.MakePeerUser(userID),
		PeerId: peer.ToPeer(),
		Date:   row.Date,
		Action: mtproto.MakeTLMessageActionHistoryClear(nil).To_MessageAction(),
	}).To_Message()
}

func historyClearJSON(msg *mtproto.Message) (string, error) {
	raw, err := jsonx.Marshal(msg)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// historyStore is the slice of storage messages.deleteHistory needs.
// A failed Revoke or Delete leaves the previous slices committed.
type historyStore interface {
	List(ctx context.Context, userID int64, peer *mtproto.PeerUtil, maxID, minDate, maxDate, limit int32) (rows []historyRow, total int32, err error)
	TopID(ctx context.Context, userID int64, peer *mtproto.PeerUtil) (int32, error)
	Revoke(ctx context.Context, fromID int64, peer *mtproto.PeerUtil, dataIDs []int64) error
	Delete(ctx context.Context, userID int64, ids []int32) error
	Rewrite(ctx context.Context, userID int64, peer *mtproto.PeerUtil, row historyRow) (*mtproto.Message, error)
	Touch(ctx context.Context, userID int64, peer *mtproto.PeerUtil) error
	CurrentPts(ctx context.Context, userID int64) int32
	AllocPts(ctx context.Context, userID int64, n int32) (int32, error)
	Persist(ctx context.Context, userID int64, pts, count int32, upd *mtproto.Update) error
	Push(ctx context.Context, userID, authKeyID int64, upds []*mtproto.Update) error
}

type historyRequest struct {
	UserID       int64
	AuthKeyID    int64
	Peer         *mtproto.PeerUtil
	JustClear    bool
	Revoke       bool
	MaxID        int32
	MinDate      int32
	MaxDate      int32
	Batch        int
	BeforeCommit func() error
}

func deleteHistoryOnce(ctx context.Context, st historyStore, in historyRequest) (*mtproto.Messages_AffectedHistory, error) {
	batch := in.Batch
	if batch < 1 {
		batch = deleteHistoryBatch
	}
	hasDate := in.MinDate != 0 || in.MaxDate != 0
	protect := in.JustClear && !hasDate
	if protect && in.MaxID > 0 {
		top, err := st.TopID(ctx, in.UserID, in.Peer)
		if err != nil {
			return nil, err
		}
		if top > in.MaxID {
			protect = false
		}
	}
	limit := int32(batch)
	if protect {
		limit++
	}
	rows, total, err := st.List(ctx, in.UserID, in.Peer, in.MaxID, in.MinDate, in.MaxDate, limit)
	if err != nil {
		return nil, err
	}
	plan := planHistoryDelete(total, rows, batch, protect)
	if len(plan.Delete) == 0 && plan.Rewrite == nil {
		return affectedHistory(st.CurrentPts(ctx, in.UserID), 0, 0), nil
	}
	if in.BeforeCommit != nil {
		if err = in.BeforeCommit(); err != nil {
			return nil, err
		}
	}
	if in.Revoke && in.Peer != nil && in.UserID != in.Peer.PeerId && len(plan.Delete) > 0 {
		dataIDs := make([]int64, len(plan.Delete))
		for i, row := range plan.Delete {
			dataIDs[i] = row.DataID
		}
		if err = st.Revoke(ctx, in.UserID, in.Peer, dataIDs); err != nil {
			return nil, err
		}
	}
	if len(plan.Delete) > 0 {
		ids := make([]int32, len(plan.Delete))
		for i, row := range plan.Delete {
			ids[i] = row.BoxID
		}
		if err = st.Delete(ctx, in.UserID, ids); err != nil {
			return nil, err
		}
	}
	var clear *mtproto.Message
	if plan.Rewrite != nil {
		clear, err = st.Rewrite(ctx, in.UserID, in.Peer, *plan.Rewrite)
		if err != nil {
			return nil, err
		}
	}
	if err = st.Touch(ctx, in.UserID, in.Peer); err != nil {
		return nil, err
	}
	ups, ptsCount := historyUpdates(in.Peer, plan, clear, 0)
	if ptsCount == 0 {
		return affectedHistory(st.CurrentPts(ctx, in.UserID), 0, plan.Offset), nil
	}
	pts, err := st.AllocPts(ctx, in.UserID, ptsCount)
	if err != nil {
		return nil, err
	}
	ups, ptsCount = historyUpdates(in.Peer, plan, clear, pts)
	for _, upd := range ups {
		if err = st.Persist(ctx, in.UserID, upd.GetPts_INT32(), upd.GetPtsCount(), upd); err != nil {
			return nil, err
		}
	}
	if err = st.Push(ctx, in.UserID, in.AuthKeyID, ups); err != nil {
		return nil, err
	}
	return affectedHistory(pts, ptsCount, plan.Offset), nil
}

func affectedHistory(pts, ptsCount, offset int32) *mtproto.Messages_AffectedHistory {
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: ptsCount,
		Offset:   offset,
	}).To_Messages_AffectedHistory()
}
