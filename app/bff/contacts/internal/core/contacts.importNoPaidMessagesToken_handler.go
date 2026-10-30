package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
)

// ContactsImportNoPaidMessagesToken
// contacts.importNoPaidMessagesToken#35a7c6ca user_id:InputUser token:string = User;
//
// Grants the caller a free-message exception for user_id when token was
// issued by that user. An unknown, revoked, expired, or mismatched token
// returns NO_PAID_MESSAGES_TOKEN_INVALID, which clients render as
// "This free messages link is invalid or has expired."
func (c *ContactsCore) ContactsImportNoPaidMessagesToken(in *mtproto.TLContactsImportNoPaidMessagesToken) (*mtproto.User, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if in.GetUserId() == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	peer := mtproto.FromInputUser(c.MD.UserId, in.GetUserId())
	if peer == nil || !peer.IsUser() || peer.PeerId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	token := dao.NormalizeNoPaidToken(in.GetToken())
	if token == "" {
		return nil, mtproto.ErrNoPaidMessagesTokenInvalid
	}
	owner, err := c.svcCtx.Dao.Tokens.LookupNoPaid(c.ctx, token, peer.PeerId)
	if err != nil || owner == 0 {
		c.Logger.Errorf("contacts.importNoPaidMessagesToken - NO_PAID_MESSAGES_TOKEN_INVALID")
		return nil, mtproto.ErrNoPaidMessagesTokenInvalid
	}
	if c.MD.UserId != owner {
		if err := c.svcCtx.Dao.Tokens.GrantNoPaid(c.ctx, owner, c.MD.UserId); err != nil {
			c.Logger.Errorf("contacts.importNoPaidMessagesToken - error: %v", err)
			return nil, mtproto.ErrInternalServerError
		}
	}
	user, err := c.loadUser(c.MD.UserId, owner)
	if err != nil {
		c.Logger.Errorf("contacts.importNoPaidMessagesToken - error: %v", err)
		if err == mtproto.ErrInputUserDeactivated {
			return nil, mtproto.ErrNoPaidMessagesTokenInvalid
		}
		return nil, err
	}
	return user, nil
}
