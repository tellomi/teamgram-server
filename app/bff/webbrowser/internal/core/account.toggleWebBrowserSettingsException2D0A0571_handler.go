package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// AccountToggleWebBrowserSettingsException2D0A0571
// Older constructor. Layer 232 uses #60ed4229 and returns Updates.
// This variant returns Bool after applying the same exception change.
func (c *WebBrowserCore) AccountToggleWebBrowserSettingsException2D0A0571(in *mtproto.TLAccountToggleWebBrowserSettingsException2D0A0571) (*mtproto.Bool, error) {
	if in == nil {
		in = &mtproto.TLAccountToggleWebBrowserSettingsException2D0A0571{}
	}
	if _, err := accountrest.ToggleWebBrowserException(c.userID(), in.GetUrl(), in.GetDelete(), in.GetOpenExternalBrowser()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
