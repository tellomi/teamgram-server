package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// Telegram service accounts. A first message from one of these is not a
// stranger's private message, so the add-contact bar stays off.
const (
	telegramServiceUser int64 = 777000
	telegramNotifyUser  int64 = 42777
	telegramSpamBot     int64 = 424000
)

// strangerBarWanted is the bar Telegram shows the recipient of the first
// private message from someone who is not a contact and not a bot.
func strangerBarWanted(from int64, contact, bot bool) bool {
	switch from {
	case 0, telegramServiceUser, telegramNotifyUser, telegramSpamBot:
		return false
	}
	return !contact && !bot
}

func strangerPeerSettings() *mtproto.PeerSettings {
	return mtproto.MakeTLPeerSettings(&mtproto.PeerSettings{
		ReportSpam:   true,
		AddContact:   true,
		BlockContact: true,
	}).To_PeerSettings()
}

// openStrangerBar stores report_spam, add_contact, and block_contact for the
// recipient of a new private dialog. A dialog that already exists is left
// alone, so hidePeerSettingsBar, add-contact, and report stay cleared.
// A user-service error does not fail the message.
func (c *InboxCore) openStrangerBar(owner, from int64, peerType int32, existed bool) *mtproto.PeerSettings {
	if existed || peerType != mtproto.PEER_USER || owner == 0 || from == 0 || owner == from {
		return nil
	}
	if !strangerBarWanted(from, false, false) {
		return nil
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil
	}
	if bot, err := c.svcCtx.Dao.UserClient.UserIsBot(c.ctx, &user.TLUserIsBot{Id: from}); err != nil {
		c.Logger.Errorf("peer settings bar - bot: %v", err)
		return nil
	} else if mtproto.FromBool(bot) {
		return nil
	}
	contact, err := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &user.TLUserCheckContact{UserId: owner, Id: from})
	if err != nil {
		c.Logger.Errorf("peer settings bar - contact: %v", err)
		return nil
	}
	if mtproto.FromBool(contact) || !strangerBarWanted(from, false, false) {
		return nil
	}
	settings := strangerPeerSettings()
	if _, err = c.svcCtx.Dao.UserClient.UserAddPeerSettings(c.ctx, &user.TLUserAddPeerSettings{
		UserId:   owner,
		PeerType: mtproto.PEER_USER,
		PeerId:   from,
		Settings: settings,
	}); err != nil {
		c.Logger.Errorf("peer settings bar - store: %v", err)
		return nil
	}
	return settings
}
