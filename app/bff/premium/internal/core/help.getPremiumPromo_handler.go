package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/langpack"
)

// HelpGetPremiumPromo
// help.getPremiumPromo#b81b93d4 = help.PremiumPromo;
func (c *PremiumCore) HelpGetPremiumPromo(in *mtproto.TLHelpGetPremiumPromo) (*mtproto.Help_PremiumPromo, error) {
	_ = in
	return PremiumPromo(c.clientLang()), nil
}

func (c *PremiumCore) clientLang() string {
	if c == nil || c.MD == nil {
		return "en"
	}
	return langpack.ClientLang(c.MD.GetLangCode(), c.MD.GetSystemLangCode(), c.MD.GetLangpack())
}
