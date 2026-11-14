package mysql_dao

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/logx"
)

const (
	topicNotifySelectColumns = "id, user_id, peer_type, peer_id, top_msg_id, show_previews, silent, mute_until, sound, stories_muted, stories_hide_sender, ios_sound, android_sound, other_sound, stories_ios_sound, stories_android_sound, stories_other_sound"
	// topicNotifyCacheTargetSQL includes soft-deleted rows so reset can still
	// name the Redis keys a previous attempt left behind.
	topicNotifyCacheTargetSQL = "select peer_type, peer_id, top_msg_id from user_notify_topic_settings where user_id = ?"
)

// UserNotifyTopicSettingsDAO stores inputNotifyForumTopic rows.
// The dialog table is untouched: a topic does not share its unique key.
type UserNotifyTopicSettingsDAO struct {
	db *sqlx.DB
}

func NewUserNotifyTopicSettingsDAO(db *sqlx.DB) *UserNotifyTopicSettingsDAO {
	return &UserNotifyTopicSettingsDAO{db: db}
}

// InsertOrUpdateExt inserts or revives one topic row.
// cMap is the partial update. insertOnly fills columns only on the first insert.
func (dao *UserNotifyTopicSettingsDAO) InsertOrUpdateExt(ctx context.Context, userId int64, peerType int32, peerId int64, topMsgId int32, cMap map[string]interface{}, insertOnly map[string]interface{}) (lastInsertId, rowsAffected int64, err error) {
	var (
		s1 []string
		s2 []string
		s3 []string
		r  sql.Result
	)
	for k := range cMap {
		s1 = append(s1, k)
		s2 = append(s2, ":"+k)
		s3 = append(s3, fmt.Sprintf("%s = :%s", k, k))
	}
	for k, v := range insertOnly {
		if _, exists := cMap[k]; exists {
			continue
		}
		s1 = append(s1, k)
		s2 = append(s2, ":"+k)
		cMap[k] = v
	}
	cMap["user_id"] = userId
	cMap["peer_type"] = peerType
	cMap["peer_id"] = peerId
	cMap["top_msg_id"] = topMsgId

	query := `
		insert into user_notify_topic_settings
			(user_id, peer_type, peer_id, top_msg_id, %s)
		values
			(:user_id, :peer_type, :peer_id, :top_msg_id, %s)
		on duplicate key update %s`
	update := "deleted = 0"
	if len(s3) > 0 {
		update = strings.Join(s3, ", ") + ", deleted = 0"
	}
	ss := fmt.Sprintf(query, strings.Join(s1, ","), strings.Join(s2, ","), update)
	r, err = dao.db.NamedExec(ctx, ss, cMap)
	if err != nil {
		logx.WithContext(ctx).Errorf("namedExec in InsertOrUpdateTopic(%v), error: %v", cMap, err)
		return
	}
	lastInsertId, err = r.LastInsertId()
	if err != nil {
		logx.WithContext(ctx).Errorf("lastInsertId in InsertOrUpdateTopic(%v), error: %v", cMap, err)
		return
	}
	rowsAffected, err = r.RowsAffected()
	if err != nil {
		logx.WithContext(ctx).Errorf("rowsAffected in InsertOrUpdateTopic(%v), error: %v", cMap, err)
	}
	return
}

func (dao *UserNotifyTopicSettingsDAO) Select(ctx context.Context, userId int64, peerType int32, peerId int64, topMsgId int32) (rValue *dataobject.UserNotifySettingsDO, err error) {
	do := &dataobject.UserNotifySettingsDO{}
	query := "select " + topicNotifySelectColumns + " from user_notify_topic_settings where user_id = ? and peer_type = ? and peer_id = ? and top_msg_id = ? and deleted = 0"
	err = dao.db.QueryRowPartial(ctx, do, query, userId, peerType, peerId, topMsgId)
	if err != nil {
		if !errors.Is(err, sqlx.ErrNotFound) {
			logx.WithContext(ctx).Errorf("queryx in SelectTopic(_), error: %v", err)
			return
		}
		err = nil
		return
	}
	rValue = do
	return
}

func (dao *UserNotifyTopicSettingsDAO) SelectAll(ctx context.Context, userId int64) (rList []dataobject.UserNotifySettingsDO, err error) {
	var values []dataobject.UserNotifySettingsDO
	query := "select " + topicNotifySelectColumns + " from user_notify_topic_settings where user_id = ? and deleted = 0"
	err = dao.db.QueryRowsPartial(ctx, &values, query, userId)
	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectAllTopic(_), error: %v", err)
		return
	}
	rList = values
	return
}

func (dao *UserNotifyTopicSettingsDAO) SelectCacheTargets(ctx context.Context, userId int64) (rList []dataobject.UserNotifySettingsDO, err error) {
	var values []dataobject.UserNotifySettingsDO
	err = dao.db.QueryRowsPartial(ctx, &values, topicNotifyCacheTargetSQL, userId)
	if err != nil {
		logx.WithContext(ctx).Errorf("queryx in SelectTopicCacheTargets(_), error: %v", err)
		return
	}
	rList = values
	return
}

func (dao *UserNotifyTopicSettingsDAO) DeleteAll(ctx context.Context, userId int64) (rowsAffected int64, err error) {
	r, err := dao.db.Exec(ctx, "update user_notify_topic_settings set deleted = 1 where user_id = ?", userId)
	if err != nil {
		logx.WithContext(ctx).Errorf("exec in DeleteAllTopic(_), error: %v", err)
		return
	}
	rowsAffected, err = r.RowsAffected()
	if err != nil {
		logx.WithContext(ctx).Errorf("rowsAffected in DeleteAllTopic(_), error: %v", err)
	}
	return
}
