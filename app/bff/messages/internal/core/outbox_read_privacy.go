package core

import (
	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// outboxReadPrivacy applies globalPrivacySettings.hide_read_marks.
//
// https://core.telegram.org/method/messages.getOutboxReadDate
// https://core.telegram.org/constructor/globalPrivacySettings
//
// The peer's hide_read_marks sets userFull.read_dates_private. A caller who
// cannot see that read date gets USER_PRIVACY_RESTRICTED. The caller who
// turned the setting on also cannot fetch anyone else's read date, unless
// they have Premium: that case is YOUR_PRIVACY_RESTRICTED.
func (c *MessagesCore) outboxReadPrivacy(peerID int64) error {
	if c == nil || c.MD == nil || peerID == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil
	}
	selfHides, err := c.hideReadMarks(c.MD.UserId)
	if err != nil {
		return err
	}
	if selfHides && !c.callerPremium() {
		return mtproto.ErrYourPrivacyRestricted
	}
	peerHides, err := c.hideReadMarks(peerID)
	if err != nil {
		return err
	}
	if peerHides {
		return mtproto.ErrUserPrivacyRestricted
	}
	return nil
}

func (c *MessagesCore) hideReadMarks(userID int64) (bool, error) {
	settings, err := c.svcCtx.Dao.UserClient.UserGetGlobalPrivacySettings(c.ctx, &userpb.TLUserGetGlobalPrivacySettings{UserId: userID})
	if err != nil {
		return false, err
	}
	return settings.GetHideReadMarks(), nil
}

func (c *MessagesCore) callerPremium() bool {
	me, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: c.MD.UserId})
	return err == nil && me.Premium()
}
