package mysql_dao

import (
	"context"
	"database/sql"
	"errors"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	topicDraftUpsertSQL     = "insert into dialog_topic_drafts (user_id, peer_type, peer_id, top_msg_id, draft_type, draft_message_data, date2) values (?, ?, ?, ?, ?, ?, ?) on duplicate key update draft_type = values(draft_type), draft_message_data = values(draft_message_data), date2 = values(date2)"
	topicDraftSelectUserSQL = "select id, user_id, peer_type, peer_id, top_msg_id, draft_type, draft_message_data, date2 from dialog_topic_drafts where user_id = ? and draft_type > 0"
	topicDraftClearOneSQL   = "update dialog_topic_drafts set draft_type = 1, draft_message_data = ?, date2 = ? where user_id = ? and peer_type = ? and peer_id = ? and top_msg_id = ? and draft_type = 2"
	topicDraftClearUserSQL  = "update dialog_topic_drafts set draft_type = 0, draft_message_data = 'null' where user_id = ? and draft_type > 0"
)

// TopicDraftsDAO stores forum-topic drafts.
// The dialogs row is not written here.
type TopicDraftsDAO struct {
	db *sqlx.DB
}

func NewTopicDraftsDAO(db *sqlx.DB) *TopicDraftsDAO {
	return &TopicDraftsDAO{db: db}
}

func (dao *TopicDraftsDAO) Upsert(ctx context.Context, do *dataobject.DialogTopicDraftDO) error {
	if dao == nil || dao.db == nil || do == nil || do.TopMsgId <= 0 {
		return nil
	}
	_, err := dao.db.Exec(ctx, topicDraftUpsertSQL, do.UserId, do.PeerType, do.PeerId, do.TopMsgId, do.DraftType, do.DraftMessageData, do.Date2)
	if err != nil {
		logx.WithContext(ctx).Errorf("exec in TopicDraftUpsert(%v), error: %v", do, err)
	}
	return err
}

func (dao *TopicDraftsDAO) SelectByUser(ctx context.Context, userId int64) ([]dataobject.DialogTopicDraftDO, error) {
	if dao == nil || dao.db == nil {
		return nil, nil
	}
	var values []dataobject.DialogTopicDraftDO
	err := dao.db.QueryRowsPartial(ctx, &values, topicDraftSelectUserSQL, userId)
	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in TopicDraftSelectByUser(_), error: %v", err)
		return nil, err
	}
	return values, nil
}

// ClearOne marks one topic draft empty. Rows affected is 0 when that topic
// had no stored draft, so the caller does not push a clear for it.
func (dao *TopicDraftsDAO) ClearOne(ctx context.Context, userId int64, peerType int32, peerId int64, topMsgId int32, data string, date2 int64) (int64, error) {
	if dao == nil || dao.db == nil || topMsgId <= 0 {
		return 0, nil
	}
	res, err := dao.db.Exec(ctx, topicDraftClearOneSQL, data, date2, userId, peerType, peerId, topMsgId)
	if err != nil {
		logx.WithContext(ctx).Errorf("exec in TopicDraftClearOne(_), error: %v", err)
		return 0, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		logx.WithContext(ctx).Errorf("rowsAffected in TopicDraftClearOne(_), error: %v", err)
	}
	return n, err
}

func (dao *TopicDraftsDAO) ClearByUser(ctx context.Context, userId int64) error {
	if dao == nil || dao.db == nil {
		return nil
	}
	_, err := dao.db.Exec(ctx, topicDraftClearUserSQL, userId)
	if err != nil {
		logx.WithContext(ctx).Errorf("exec in TopicDraftClearByUser(_), error: %v", err)
	}
	return err
}

// Select is used by tests that want one row. A missing row is (nil, nil).
func (dao *TopicDraftsDAO) Select(ctx context.Context, userId int64, peerType int32, peerId int64, topMsgId int32) (*dataobject.DialogTopicDraftDO, error) {
	if dao == nil || dao.db == nil {
		return nil, nil
	}
	do := &dataobject.DialogTopicDraftDO{}
	err := dao.db.QueryRowPartial(ctx, do, "select id, user_id, peer_type, peer_id, top_msg_id, draft_type, draft_message_data, date2 from dialog_topic_drafts where user_id = ? and peer_type = ? and peer_id = ? and top_msg_id = ?", userId, peerType, peerId, topMsgId)
	if err != nil {
		if !errors.Is(err, sqlx.ErrNotFound) && !errors.Is(err, sql.ErrNoRows) {
			logx.WithContext(ctx).Errorf("queryx in TopicDraftSelect(_), error: %v", err)
			return nil, err
		}
		return nil, nil
	}
	return do, nil
}
