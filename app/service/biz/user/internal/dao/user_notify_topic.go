package dao

import (
	"context"
	"errors"
	"fmt"

	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/internal/dal/dataobject"
)

// TopicNotifySettingsCacheKey is the Redis key for one forum topic.
// It extends the dialog key user_notify_settings_v2_{user}_{type}_{peer}
// with the topic id, so the two cannot collide.
func TopicNotifySettingsCacheKey(userID int64, peerType int32, peerID int64, topMsgID int32) string {
	return fmt.Sprintf("%s_%d_%d_%d_%d", userNotifySettingsKeyPrefix, userID, peerType, peerID, topMsgID)
}

func (d *Dao) GetUserTopicNotifySettings(ctx context.Context, id int64, peerType int32, peerId int64, topMsgId int32) (*mtproto.PeerNotifySettings, error) {
	settings := mtproto.MakeTLPeerNotifySettings(nil).To_PeerNotifySettings()
	err := d.CachedConn.QueryRow(
		ctx,
		settings,
		TopicNotifySettingsCacheKey(id, peerType, peerId, topMsgId),
		func(ctx context.Context, conn *sqlx.DB, v interface{}) error {
			do, err := d.UserNotifyTopicSettingsDAO.Select(ctx, id, peerType, peerId, topMsgId)
			if err != nil {
				return err
			}
			if do != nil {
				setPeerNotifySettingsByDO(v.(*mtproto.PeerNotifySettings), do)
			}
			return nil
		})
	if err != nil {
		return nil, err
	}
	return settings, nil
}

func (d *Dao) SetUserTopicNotifySettings(ctx context.Context, id int64, peerType int32, peerId int64, topMsgId int32, settings *mtproto.PeerNotifySettings) error {
	_, _, err := d.CachedConn.Exec(
		ctx,
		func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
			updates, insertOnly := notifyWriteColumns(settings)
			return d.UserNotifyTopicSettingsDAO.InsertOrUpdateExt(ctx, id, peerType, peerId, topMsgId, updates, insertOnly)
		},
		TopicNotifySettingsCacheKey(id, peerType, peerId, topMsgId))
	return err
}

// ListTopicNotifySettings returns live topic rows. A database that has not
// applied the additive migration yet has no table; dialogs still load.
func (d *Dao) ListTopicNotifySettings(ctx context.Context, userID int64) ([]dataobject.UserNotifySettingsDO, error) {
	rows, err := d.UserNotifyTopicSettingsDAO.SelectAll(ctx, userID)
	if err != nil && mysqlMissingTable(err) {
		return nil, nil
	}
	return rows, err
}

// TopicNotifyCacheKeys names the Redis entries reset must drop.
// A row with no topic id is not a topic and is left to the dialog key.
func TopicNotifyCacheKeys(userID int64, rows []dataobject.UserNotifySettingsDO) []string {
	seen := make(map[string]struct{}, len(rows))
	keys := make([]string, 0, len(rows))
	for _, row := range rows {
		if row.TopMsgId <= 0 {
			continue
		}
		key := TopicNotifySettingsCacheKey(userID, row.PeerType, row.PeerId, row.TopMsgId)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

// ResetTopicNotifySettings soft-deletes every topic row and drops its cache
// key. Already-deleted rows stay in the key list so a retry can finish a
// reset whose cache delete failed.
func (d *Dao) ResetTopicNotifySettings(ctx context.Context, userID int64) error {
	rows, err := d.UserNotifyTopicSettingsDAO.SelectCacheTargets(ctx, userID)
	if err != nil {
		if mysqlMissingTable(err) {
			return nil
		}
		return err
	}
	if _, err = d.UserNotifyTopicSettingsDAO.DeleteAll(ctx, userID); err != nil {
		if mysqlMissingTable(err) {
			return nil
		}
		return err
	}
	keys := TopicNotifyCacheKeys(userID, rows)
	if len(keys) == 0 {
		return nil
	}
	return d.CachedConn.DelCache(ctx, keys...)
}

func mysqlMissingTable(err error) bool {
	var me *mysqldriver.MySQLError
	return errors.As(err, &me) && me.Number == 1146
}
