package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
)

// pushQuickReplyNotMe delivers a new or appended quick-reply message to the
// account's other sessions. The calling session already has the RPC result.
// sync.updatesNotMe skips its auth key, the same path account.updateNotifySettings
// uses. A push failure leaves the stored shortcut in place: failing the RPC
// would make the client send the shortcut message again.
func (c *MessagesCore) pushQuickReplyNotMe(ups *mtproto.Updates) {
	if c == nil || c.MD == nil || c.MD.UserId == 0 || ups == nil || len(ups.GetUpdates()) == 0 {
		return
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.SyncClient == nil {
		return
	}
	_, err := c.svcCtx.Dao.SyncClient.SyncUpdatesNotMe(c.ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        c.MD.UserId,
		PermAuthKeyId: c.MD.PermAuthKeyId,
		Updates:       ups,
	})
	if err != nil && c.Logger != nil {
		c.Logger.Errorf("quick reply sync: %v", err)
	}
}
