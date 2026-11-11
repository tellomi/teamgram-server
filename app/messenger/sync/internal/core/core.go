/*
 * Created from 'scheme.tl' by 'mtprotoc'
 *
 * Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
 *  All rights reserved.
 *
 * Author: teamgramio (teamgram.io@gmail.com)
 */

package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/interface/session/session"
	"github.com/teamgram/teamgram-server/app/messenger/sync/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/authsession/authsession"
	"github.com/teamgram/teamgram-server/app/service/status/status"

	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type SyncType int

const (
	syncTypeUser      SyncType = 1 // 该用户所有设备
	syncTypeUserNotMe SyncType = 2 // 该用户除了某个设备
	syncTypeUserMe    SyncType = 3 // 该用户指定某个设备
)

type SyncCore struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
	MD *metadata.RpcMetadata
}

func New(ctx context.Context, svcCtx *svc.ServiceContext) *SyncCore {
	return &SyncCore{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
		MD:     metadata.RpcMetadataFromIncoming(ctx),
	}
}

func (c *SyncCore) processUpdates(syncType SyncType, userId int64, isBot bool, ups *mtproto.Updates) (needPush bool, err error) {
	mtproto.VisitUpdates(userId, ups, map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updateNewMessage: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			// c.svcCtx.Dao.AddToPtsQueue(c.ctx, userId, update.Pts_INT32, update.PtsCount, update)
			// removed in pure push mode
			needPush = true
		},
		mtproto.Predicate_updateDeleteMessages: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			// c.svcCtx.Dao.AddToPtsQueue(c.ctx, userId, update.Pts_INT32, update.PtsCount, update)
			// removed in pure push mode
			needPush = true
		},
		mtproto.Predicate_updateReadHistoryInbox: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			// c.svcCtx.Dao.AddToPtsQueue(c.ctx, userId, update.Pts_INT32, update.PtsCount, update)
			// removed in pure push mode
			needPush = true
		},
		mtproto.Predicate_updateReadHistoryOutbox: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			// c.svcCtx.Dao.AddToPtsQueue(c.ctx, userId, update.Pts_INT32, update.PtsCount, update)
			// removed in pure push mode
			needPush = true
		},
		mtproto.Predicate_updateWebPage: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			// c.svcCtx.Dao.AddToPtsQueue(c.ctx, userId, update.Pts_INT32, update.PtsCount, update)
			// removed in pure push mode
			needPush = true
		},
		mtproto.Predicate_updateReadMessagesContents: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			// c.svcCtx.Dao.AddToPtsQueue(c.ctx, userId, update.Pts_INT32, update.PtsCount, update)
			// removed in pure push mode
			needPush = true
		},
		mtproto.Predicate_updateEditMessage: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			// c.svcCtx.Dao.AddToPtsQueue(c.ctx, userId, update.Pts_INT32, update.PtsCount, update)
			// removed in pure push mode
			needPush = true
		},
		mtproto.Predicate_updateFolderPeers: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			if syncType == syncTypeUserNotMe {
				// The folders handler already stored this pts. Sync only delivers it.
			}
			// Deliver the archive change to the user's other sessions. pts_count 1
			// is replayed by updates.getDifference; pts_count 0 is applied in place.
			needPush = true
		},
		mtproto.Predicate_updateDialogFilter: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
		mtproto.Predicate_updateDialogFilterOrder: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
		mtproto.Predicate_updatePinnedMessages: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			if syncType == syncTypeUserNotMe {
				// c.svcCtx.Dao.AddToPtsQueue(c.ctx, userId, update.Pts_INT32, update.PtsCount, update)
				// removed in pure push mode
			}
		},
		mtproto.Predicate_updatePhoneCall: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			if update.GetPhoneCall().GetPredicateName() == mtproto.Predicate_phoneCallRequested {
				// log.Debugf("recv phoneCallRequested")
				needPush = true
			}
		},
		mtproto.Predicate_updateMessageReactions: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			// The constructor has no pts. Mark it for delivery anyway so every
			// online session of the message author is told. getDifference
			// carries the same update from the reaction store, not from pts.
			needPush = true
		},
		mtproto.Predicate_updatePeerSettings: func(
			userId int64,
			update *mtproto.Update,
			users []*mtproto.User,
			chats []*mtproto.Chat,
			date int32,
		) {
			needPush = true
		},
	})

	return needPush, nil
}

func (c *SyncCore) pushUpdatesToSession(syncType SyncType, userId, permAuthKeyId int64, hasServerId *wrapperspb.StringValue, authKeyId, sessionId *wrapperspb.Int64Value, pushData *mtproto.Updates, notification bool) {
	if syncType == syncTypeUserMe && hasServerId != nil {
		c.Logger.Debugf("pushUpdatesToSession - pushData: {server_id: %v, auth_key_id: %v}", hasServerId, authKeyId)
		keyID := permAuthKeyId
		if keyID == 0 && authKeyId != nil {
			keyID = authKeyId.GetValue()
		}
		if !c.authKeyServes(userId, keyID) {
			return
		}
		if sessionId != nil {
			msg := &session.TLSessionPushSessionUpdatesData{
				PermAuthKeyId: permAuthKeyId,
				AuthKeyId:     authKeyId.GetValue(),
				SessionId:     sessionId.GetValue(),
				Updates:       pushData,
			}
			session.SetPushSessionUpdatesUser(msg, userId)
			_ = c.svcCtx.Dao.PushSessionUpdatesToSession(c.ctx, hasServerId.GetValue(), msg)
		} else {
			c.pushUpdates(hasServerId.GetValue(), permAuthKeyId, notification, pushData, userId)
		}
	} else {
		var (
			pushExcludeList   = make([]int64, 0)
			serverIdKeyIdList = make(map[string][]int64)
		)

		statusList, _ := c.svcCtx.Dao.StatusClient.StatusGetUserOnlineSessions(c.ctx, &status.TLStatusGetUserOnlineSessions{
			UserId: userId,
		})
		c.Logger.Debugf("statusList - #%v", statusList)
		for _, sess := range statusList.GetUserSessions() {
			if sess == nil || !c.authKeyServes(userId, sess.AuthKeyId) {
				continue
			}
			if syncType == syncTypeUserNotMe && sess.AuthKeyId == permAuthKeyId {
				continue
			}
			pushExcludeList = append(pushExcludeList, sess.PermAuthKeyId)
			if keyIdList, ok := serverIdKeyIdList[sess.Gateway]; ok {
				keyIdList = append(keyIdList, sess.AuthKeyId)
				serverIdKeyIdList[sess.Gateway] = keyIdList
			} else {
				serverIdKeyIdList[sess.Gateway] = []int64{sess.AuthKeyId}
			}
		}

		c.Logger.Debugf("serverIdKeyIdList - #%v", serverIdKeyIdList)
		for serverId, keyIdList := range serverIdKeyIdList {
			for _, keyId := range keyIdList {
				c.pushUpdates(serverId, keyId, notification, pushData, userId)
			}
		}

		if syncType == syncTypeUser {
			if c.svcCtx.Dao.PushClient != nil {
				c.Logger.Debugf("push PushClient...")
				_, _ = c.svcCtx.Dao.PushClient.SyncPushUpdatesIfNot(c.ctx, &sync.TLSyncPushUpdatesIfNot{
					UserId:   userId,
					Excludes: pushExcludeList,
					Updates:  pushData,
				})
			}
		}
	}
}

// pushUpdatesExcept delivers to every online session whose auth key or
// permanent auth key is not in excludes. It does not publish another
// pushUpdatesIfNot, so a consumer of that method cannot loop.
func (c *SyncCore) pushUpdatesExcept(userId int64, excludes []int64, pushData *mtproto.Updates, notification bool) {
	skip := make(map[int64]struct{}, len(excludes))
	for _, id := range excludes {
		if id != 0 {
			skip[id] = struct{}{}
		}
	}
	statusList, _ := c.svcCtx.Dao.StatusClient.StatusGetUserOnlineSessions(c.ctx, &status.TLStatusGetUserOnlineSessions{
		UserId: userId,
	})
	for _, sess := range statusList.GetUserSessions() {
		if sess == nil || !c.authKeyServes(userId, sess.AuthKeyId) {
			continue
		}
		if _, ok := skip[sess.AuthKeyId]; ok {
			continue
		}
		if _, ok := skip[sess.PermAuthKeyId]; ok {
			continue
		}
		c.pushUpdates(sess.Gateway, sess.AuthKeyId, notification, pushData, userId)
	}
}

// pushUpdates stamps the expected user and sends one session push.
// authKeyServes has already accepted the key for this user.
func (c *SyncCore) pushUpdates(serverID string, authKeyID int64, notification bool, pushData *mtproto.Updates, userID int64) {
	msg := &session.TLSessionPushUpdatesData{
		PermAuthKeyId: authKeyID,
		Notification:  notification,
		Updates:       pushData,
	}
	session.SetPushUpdatesUser(msg, userID)
	_ = c.svcCtx.Dao.PushUpdatesToSession(c.ctx, serverID, msg)
}

// authKeyServes reports whether this auth key may receive updates for userID.
// A known owner other than userID is a stale online-session field: it is
// removed and the push is not sent. An unknown owner (authsession down, or
// no client configured) keeps the previous route.
func (c *SyncCore) authKeyServes(userID, authKeyID int64) bool {
	owner, ok := c.authKeyOwner(authKeyID)
	if !ok || owner == userID {
		return true
	}
	c.Logger.Errorf("pushUpdatesToSession - authKey %d belongs to %d, not %d; evict stale route", authKeyID, owner, userID)
	if userID > 0 && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.StatusClient != nil {
		_, _ = c.svcCtx.Dao.StatusClient.StatusSetSessionOffline(c.ctx, &status.TLStatusSetSessionOffline{
			UserId:    userID,
			AuthKeyId: authKeyID,
		})
	}
	return false
}

func (c *SyncCore) authKeyOwner(authKeyID int64) (int64, bool) {
	if authKeyID == 0 || c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.AuthsessionClient == nil {
		return 0, false
	}
	got, err := c.svcCtx.Dao.AuthsessionClient.AuthsessionGetUserId(c.ctx, &authsession.TLAuthsessionGetUserId{
		AuthKeyId: authKeyID,
	})
	if err != nil || got == nil || got.GetV() == 0 {
		return 0, false
	}
	return got.GetV(), true
}
