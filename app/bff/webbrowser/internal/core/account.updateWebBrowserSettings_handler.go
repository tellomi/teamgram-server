package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AccountUpdateWebBrowserSettings
// account.updateWebBrowserSettings#9adf82fe flags:# open_external_browser:flags.0?true display_close_button:flags.1?true = account.WebBrowserSettings;
func (c *WebBrowserCore) AccountUpdateWebBrowserSettings(in *mtproto.TLAccountUpdateWebBrowserSettings) (*mtproto.Account_WebBrowserSettings, error) {
	if in == nil {
		in = &mtproto.TLAccountUpdateWebBrowserSettings{}
	}
	return accountrest.UpdateWebBrowserSettings(c.userID(), in.GetOpenExternalBrowser(), in.GetDisplayCloseButton())
}
