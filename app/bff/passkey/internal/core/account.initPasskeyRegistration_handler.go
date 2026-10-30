package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AccountInitPasskeyRegistration
// account.initPasskeyRegistration#429547e8 = account.PasskeyRegistrationOptions;
// https://core.telegram.org/method/account.initPasskeyRegistration
func (c *PasskeyCore) AccountInitPasskeyRegistration(in *mtproto.TLAccountInitPasskeyRegistration) (*mtproto.Account_PasskeyRegistrationOptions, error) {
	return accountrest.InitPasskeyRegistration(c.userID())
}
