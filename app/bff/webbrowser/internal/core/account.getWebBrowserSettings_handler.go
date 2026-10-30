package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AccountGetWebBrowserSettings
// account.getWebBrowserSettings#56655768 hash:long = account.WebBrowserSettings;
func (c *WebBrowserCore) AccountGetWebBrowserSettings(in *mtproto.TLAccountGetWebBrowserSettings) (*mtproto.Account_WebBrowserSettings, error) {
	var hash int64
	if in != nil {
		hash = in.GetHash()
	}
	return accountrest.GetWebBrowserSettings(c.userID(), hash)
}

func (c *WebBrowserCore) userID() int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	return c.MD.GetUserId()
}
