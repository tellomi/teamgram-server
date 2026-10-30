package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AccountDeleteWebBrowserSettingsExceptions
// account.deleteWebBrowserSettingsExceptions#86a0765d = account.WebBrowserSettings;
func (c *WebBrowserCore) AccountDeleteWebBrowserSettingsExceptions(in *mtproto.TLAccountDeleteWebBrowserSettingsExceptions) (*mtproto.Account_WebBrowserSettings, error) {
	return accountrest.DeleteWebBrowserExceptions(c.userID())
}
