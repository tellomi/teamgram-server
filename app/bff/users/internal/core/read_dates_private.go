package core

import (
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// peerHidesReadMarks is userFull.read_dates_private.
// account.setGlobalPrivacySettings with hide_read_marks sets the flag for
// that user. Viewers then know messages.getOutboxReadDate will fail.
func (c *UsersCore) peerHidesReadMarks(userID int64) bool {
	if c == nil || userID == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return false
	}
	settings, err := c.svcCtx.Dao.UserClient.UserGetGlobalPrivacySettings(c.ctx, &userpb.TLUserGetGlobalPrivacySettings{
		UserId: userID,
	})
	if err != nil || settings == nil {
		return false
	}
	return settings.GetHideReadMarks()
}
