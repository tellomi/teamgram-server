package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AccountToggleWebBrowserSettingsException60ED4229
// account.toggleWebBrowserSettingsException#60ed4229 flags:# delete:flags.1?true open_external_browser:flags.0?Bool url:string = Updates;
func (c *WebBrowserCore) AccountToggleWebBrowserSettingsException60ED4229(in *mtproto.TLAccountToggleWebBrowserSettingsException60ED4229) (*mtproto.Updates, error) {
	if in == nil {
		in = &mtproto.TLAccountToggleWebBrowserSettingsException60ED4229{}
	}
	return accountrest.ToggleWebBrowserException(c.userID(), in.GetUrl(), in.GetDelete(), in.GetOpenExternalBrowser())
}
