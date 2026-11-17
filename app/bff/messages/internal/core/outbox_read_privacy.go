package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

// outboxReadPrivacy applies globalPrivacySettings.hide_read_marks.
//
// https://core.telegram.org/method/messages.getOutboxReadDate
// https://core.telegram.org/constructor/globalPrivacySettings
//
// The peer's hide_read_marks sets userFull.read_dates_private. A caller who
// cannot see that read date gets USER_PRIVACY_RESTRICTED. The caller who
// turned the setting on also cannot fetch anyone else's read date. That
// case is YOUR_PRIVACY_RESTRICTED, unless users.premium is an unexpired
// subscription. TELLOMI_PREMIUM_EVERYONE makes every account look Premium
// for feature limits and does not waive this.
func (c *MessagesCore) outboxReadPrivacy(peerID int64) error {
	if c == nil || c.MD == nil || peerID == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil
	}
	selfHides, err := c.hideReadMarks(c.MD.UserId)
	if err != nil {
		return err
	}
	if selfHides && !c.callerPaidPremium() {
		return mtproto.ErrYourPrivacyRestricted
	}
	peerHides, err := c.hideReadMarks(peerID)
	if err != nil {
		return err
	}
	if peerHides {
		return mtproto.ErrUserPrivacyRestricted
	}
	// Hiding "last seen" also hides the exact read time. The caller's own
	// status privacy is YOUR_PRIVACY_RESTRICTED unless they have a paid
	// Premium subscription. The peer's status privacy is USER_PRIVACY_RESTRICTED.
	// https://core.telegram.org/method/messages.getOutboxReadDate
	if !c.statusVisible(c.MD.UserId, peerID) && !c.callerPaidPremium() {
		return mtproto.ErrYourPrivacyRestricted
	}
	if !c.statusVisible(peerID, c.MD.UserId) {
		return mtproto.ErrUserPrivacyRestricted
	}
	return nil
}

func (c *MessagesCore) statusVisible(owner, viewer int64) bool {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || owner == 0 || viewer == 0 {
		return true
	}
	ok, err := c.svcCtx.Dao.UserClient.UserCheckPrivacy(c.ctx, &userpb.TLUserCheckPrivacy{
		UserId:  owner,
		KeyType: mtproto.STATUS_TIMESTAMP,
		PeerId:  viewer,
	})
	if err != nil {
		return true
	}
	return mtproto.FromBool(ok)
}

func (c *MessagesCore) hideReadMarks(userID int64) (bool, error) {
	settings, err := c.svcCtx.Dao.UserClient.UserGetGlobalPrivacySettings(c.ctx, &userpb.TLUserGetGlobalPrivacySettings{UserId: userID})
	if err != nil {
		return false, err
	}
	return settings.GetHideReadMarks(), nil
}

func (c *MessagesCore) callerPaidPremium() bool {
	me, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: c.MD.UserId})
	if err != nil || me == nil || me.User == nil || me.Deleted() {
		return false
	}
	var expire int64
	if me.User.GetPremiumExpireDate() != nil {
		expire = me.User.GetPremiumExpireDate().GetValue()
	}
	return premium.ActiveSubscription(me.User.GetPremium(), expire, time.Now().Unix())
}
