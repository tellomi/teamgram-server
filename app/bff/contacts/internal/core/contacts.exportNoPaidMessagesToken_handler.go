package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
)

// ContactsExportNoPaidMessagesToken
// contacts.exportNoPaidMessagesToken#83c586ae flags:# revoke:flags.0?true temporary:flags.1?true = ExportedNoPaidMessagesToken;
//
// Issues a link that waives the caller's paid-message fee.
// temporary lasts one minute (client copy: "Expire after 1 minute").
// revoke invalidates the previous tokens ("The old one no longer works.").
// The link uses the free= query Telegram Desktop parses as noPaidToken.
func (c *ContactsCore) ContactsExportNoPaidMessagesToken(in *mtproto.TLContactsExportNoPaidMessagesToken) (*mtproto.ExportedNoPaidMessagesToken, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if _, err := c.loadUser(c.MD.UserId, c.MD.UserId); err != nil {
		c.Logger.Errorf("contacts.exportNoPaidMessagesToken - error: %v", err)
		return nil, err
	}
	exp, err := c.svcCtx.Dao.Tokens.ExportNoPaid(c.ctx, c.MD.UserId, in.GetRevoke(), in.GetTemporary())
	if err != nil {
		c.Logger.Errorf("contacts.exportNoPaidMessagesToken - error: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	username := c.usernameOf(c.MD.UserId)
	contactToken := ""
	if username == "" {
		minExpires := exp.Expires
		if minExpires == 0 {
			minExpires = int32(time.Now().Add(dao.CarrierTTL()).Unix())
		}
		contactToken, err = c.svcCtx.Dao.Tokens.EnsureContactCarrier(c.ctx, c.MD.UserId, minExpires)
		if err != nil {
			c.Logger.Errorf("contacts.exportNoPaidMessagesToken - error: %v", err)
			return nil, mtproto.ErrInternalServerError
		}
	}
	return mtproto.MakeExportedNoPaidMessagesToken(exp.Token, dao.NoPaidLink(username, contactToken, exp.Token), exp.Expires), nil
}
