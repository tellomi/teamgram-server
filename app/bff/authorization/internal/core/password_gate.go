package core

import (
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
)

// needsCloudPassword is true when this account has a stored SRP verifier,
// or when an authorization plugin still reports that a password is required.
func (c *AuthorizationCore) needsCloudPassword(userID int64) bool {
	if userID == 0 {
		return false
	}
	has, err := password.HasPassword(c.ctx, userID)
	if err != nil {
		c.Logger.Errorf("cloud password lookup: %v", err)
		return true
	}
	if has {
		return true
	}
	return c.svcCtx != nil && c.svcCtx.Plugin != nil && c.svcCtx.Plugin.CheckSessionPasswordNeeded(c.ctx, userID)
}

// passwordNeeded records the auth key before the caller binds it and returns
// 401 SESSION_PASSWORD_NEEDED. The text has no user id. The session service
// reads the id from the auth-key binding and from user_password_login.
func (c *AuthorizationCore) passwordNeeded(userID int64) error {
	return password.NoteLogin(c.ctx, c.authKey(), userID)
}
