package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ContactsExportContactToken
// contacts.exportContactToken#f8654027 = ExportedContactToken;
//
// Generates a temporary profile link for the currently logged-in user.
// https://core.telegram.org/method/contacts.exportContactToken
// The https form is t.me/contact/<token> (https://core.telegram.org/api/links).
func (c *ContactsCore) ContactsExportContactToken(in *mtproto.TLContactsExportContactToken) (*mtproto.ExportedContactToken, error) {
	_ = in
	if c.MD == nil || c.MD.UserId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	if _, err := c.loadUser(c.MD.UserId, c.MD.UserId); err != nil {
		c.Logger.Errorf("contacts.exportContactToken - error: %v", err)
		return nil, err
	}
	token, expires, err := c.svcCtx.Dao.Tokens.ExportContact(c.ctx, c.MD.UserId)
	if err != nil {
		c.Logger.Errorf("contacts.exportContactToken - error: %v", err)
		return nil, mtproto.ErrInternalServerError
	}
	return mtproto.MakeTLExportedContactToken(&mtproto.ExportedContactToken{
		Url:     "https://t.me/contact/" + token,
		Expires: expires,
	}).To_ExportedContactToken(), nil
}

// testMutableUsers overrides the user service in package tests.
var testMutableUsers func(ctx context.Context, in *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error)

func (c *ContactsCore) mutableUsers(in *userpb.TLUserGetMutableUsersV2) (*mtproto.MutableUsers, error) {
	if testMutableUsers != nil {
		return testMutableUsers(c.ctx, in)
	}
	return c.svcCtx.Dao.UserClient.UserGetMutableUsersV2(c.ctx, in)
}

func (c *ContactsCore) loadUser(self, id int64) (*mtproto.User, error) {
	users, err := c.mutableUsers(&userpb.TLUserGetMutableUsersV2{
		Id:      []int64{self, id},
		Privacy: true,
		HasTo:   true,
		To:      []int64{id},
	})
	if err != nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	if users == nil || !users.CheckExistUser(self, id) {
		return nil, mtproto.ErrUserIdInvalid
	}
	imm, _ := users.GetImmutableUser(id)
	if imm != nil && imm.Deleted() {
		return nil, mtproto.ErrInputUserDeactivated
	}
	u, err := users.GetUnsafeUser(self, id)
	if err != nil || u == nil {
		return nil, mtproto.ErrUserIdInvalid
	}
	return u, nil
}

func (c *ContactsCore) usernameOf(id int64) string {
	users, err := c.mutableUsers(&userpb.TLUserGetMutableUsersV2{
		Id: []int64{id},
	})
	if err != nil || users == nil {
		return ""
	}
	imm, ok := users.GetImmutableUser(id)
	if !ok || imm == nil || imm.Deleted() {
		return ""
	}
	return imm.Username()
}
