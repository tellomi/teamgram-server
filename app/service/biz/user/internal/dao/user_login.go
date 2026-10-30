package dao

import (
	"database/sql"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/verify"
)

// loginColumns is how a new account is stored.
// Email accounts keep phone NULL (empty string here; the insert writes NULL)
// and a unique email. Phone accounts leave email unset.
type loginColumns struct {
	Phone       string
	Email       sql.NullString
	CountryCode string
	EmailOnly   bool
}

func loginColumnsForNewUser(rawPhone, country string) (loginColumns, error) {
	if verify.IsEmail(rawPhone) {
		email := verify.NormalizeEmail(rawPhone)
		if !verify.ValidEmail(email) || len(email) > 255 {
			return loginColumns{}, mtproto.ErrEmailInvalid
		}
		return loginColumns{
			Email:     sql.NullString{String: email, Valid: true},
			EmailOnly: true,
		}, nil
	}
	return loginColumns{
		Phone:       rawPhone,
		CountryCode: country,
	}, nil
}
