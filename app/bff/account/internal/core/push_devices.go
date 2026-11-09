package core

import "github.com/teamgram/teamgram-server/pkg/push"

// unregisterPushDevices drops push registrations for authKeyIDs.
// A store error is logged and does not fail the RPC: the auth_users row is
// already revoked, and ListByUser will not push to it.
func (c *AccountCore) unregisterPushDevices(authKeyIDs ...int64) {
	if c == nil || c.svcCtx == nil || c.MD == nil {
		return
	}
	if err := push.UnregisterSession(c.ctx, c.svcCtx.Devices, c.MD.UserId, authKeyIDs...); err != nil && c.Logger != nil {
		c.Logger.Errorf("push: unregister devices user=%d: %v", c.MD.UserId, err)
	}
}
