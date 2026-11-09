package core

import "github.com/teamgram/teamgram-server/pkg/push"

// unregisterPushDevices drops push registrations for authKeyIDs.
// A store error is logged and does not fail the RPC: the auth_users row is
// already revoked, and ListByUser will not push to it.
func (c *AuthorizationCore) unregisterPushDevices(authKeyIDs ...int64) {
	if c == nil || c.svcCtx == nil || c.MD == nil {
		return
	}
	if err := push.UnregisterSession(c.ctx, c.svcCtx.Devices, c.MD.UserId, authKeyIDs...); err != nil && c.Logger != nil {
		c.Logger.Errorf("push: unregister devices user=%d: %v", c.MD.UserId, err)
	}
}

// logoutDeviceKeys are the auth keys auth.logOut just unbound.
// A permanent key of 0 unbinds every session of the user, so every device
// row for that user goes with them. Otherwise the permanent key is removed,
// and the temp key as well when registerDevice stored the device on it.
func logoutDeviceKeys(perm, auth int64) []int64 {
	if perm == 0 {
		return []int64{0}
	}
	if auth != 0 && auth != perm {
		return []int64{perm, auth}
	}
	return []int64{perm}
}
