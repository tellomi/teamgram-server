package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/contacts/internal/dao"
)

// ContactsImportContactToken
// contacts.importContactToken#13005788 token:string = User;
//
// Obtain user info from a temporary profile link.
// https://core.telegram.org/method/contacts.importContactToken
// 400 IMPORT_TOKEN_INVALID — the token is missing, unknown, or expired.
func (c *ContactsCore) ContactsImportContactToken(in *mtproto.TLContactsImportContactToken) (*mtproto.User, error) {
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	token := dao.NormalizeContactToken(in.GetToken())
	if token == "" {
		return nil, mtproto.ErrImportTokenInvalid
	}
	owner, err := c.svcCtx.Dao.Tokens.LookupContact(c.ctx, token)
	if err != nil || owner == 0 {
		c.Logger.Errorf("contacts.importContactToken - IMPORT_TOKEN_INVALID")
		return nil, mtproto.ErrImportTokenInvalid
	}
	user, err := c.loadUser(c.MD.UserId, owner)
	if err != nil {
		if err == mtproto.ErrInputUserDeactivated {
			return nil, mtproto.ErrImportTokenInvalid
		}
		c.Logger.Errorf("contacts.importContactToken - error: %v", err)
		return nil, mtproto.ErrImportTokenInvalid
	}
	return user, nil
}
