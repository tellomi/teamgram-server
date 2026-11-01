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

// SyncPushUpdates
// sync.pushUpdates user_id:long updates:Updates = Void;
func (c *SyncCore) SyncPushUpdates(in *sync.TLSyncPushUpdates) (*mtproto.Void, error) {
	var (
		userId  = in.GetUserId()
		updates = in.GetUpdates()
	)

	notification, err := c.processUpdates(syncTypeUser, userId, false, updates)
	if err != nil {
		c.Logger.Errorf("sync.updatesNotMe - error: %v", err)
		return nil, err
	}

	c.pushUpdatesToSession(syncTypeUser, userId, 0, nil, nil, nil, updates, notification)
	if incoming := push.Messages(userId, updates); len(incoming) > 0 {
		ids := make([]int32, 0, len(incoming))
		for _, msg := range incoming {
			ids = append(ids, msg.MsgID)
		}
		// messages.receivedMessages drops these once the client confirms them.
		_ = notifyack.Shared().Note(c.ctx, userId, ids)
	}
	if c.svcCtx.Push != nil && len(push.Messages(userId, updates)) > 0 {
		go func() {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(c.ctx), 20*time.Second)
			defer cancel()
			c.svcCtx.Push.Deliver(ctx, userId, updates)
		}()
	}

	return mtproto.EmptyVoid, nil
}
