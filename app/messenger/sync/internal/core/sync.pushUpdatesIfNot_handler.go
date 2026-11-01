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
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/pkg/notifyack"
	"github.com/teamgram/teamgram-server/pkg/push"
)

// SyncPushUpdatesIfNot
// sync.pushUpdatesIfNot user_id:long excludes:Vector<long> updates:Updates = Void;
//
// Channel, story, and phone code publish this onto Sync-T when one device
// already has the update. The consumer used to drop it ("invalid key: <user id>")
// because the method was not in the switch, and this handler returned
// METHOD_NOT_IMPL. Deliver it to every other online session.
func (c *SyncCore) SyncPushUpdatesIfNot(in *sync.TLSyncPushUpdatesIfNot) (*mtproto.Void, error) {
	userId := in.GetUserId()
	updates := in.GetUpdates()

	notification, err := c.processUpdates(syncTypeUserNotMe, userId, false, updates)
	if err != nil {
		c.Logger.Errorf("sync.pushUpdatesIfNot - error: %v", err)
		return nil, err
	}

	c.pushUpdatesExcept(userId, in.GetExcludes(), updates, notification)
	noteIncoming(c, userId, updates)
	c.deliverPush(userId, updates)
	return mtproto.EmptyVoid, nil
}

// SyncPushBotUpdates
// sync.pushBotUpdates user_id:long updates:Updates = Void;
func (c *SyncCore) SyncPushBotUpdates(in *sync.TLSyncPushBotUpdates) (*mtproto.Void, error) {
	userId := in.GetUserId()
	updates := in.GetUpdates()

	notification, err := c.processUpdates(syncTypeUser, userId, true, updates)
	if err != nil {
		c.Logger.Errorf("sync.pushBotUpdates - error: %v", err)
		return nil, err
	}

	c.pushUpdatesToSession(syncTypeUser, userId, 0, nil, nil, nil, updates, notification)
	noteIncoming(c, userId, updates)
	c.deliverPush(userId, updates)
	return mtproto.EmptyVoid, nil
}

func noteIncoming(c *SyncCore, userId int64, updates *mtproto.Updates) {
	incoming := push.Messages(userId, updates)
	if len(incoming) == 0 {
		return
	}
	ids := make([]int32, 0, len(incoming))
	for _, msg := range incoming {
		ids = append(ids, msg.MsgID)
	}
	_ = notifyack.Shared().Note(c.ctx, userId, ids)
}

func (c *SyncCore) deliverPush(userId int64, updates *mtproto.Updates) {
	if c.svcCtx.Push == nil || !push.ShouldDeliver(userId, updates) {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), 20*time.Second)
		defer cancel()
		c.svcCtx.Push.Deliver(ctx, userId, updates)
	}()
}
