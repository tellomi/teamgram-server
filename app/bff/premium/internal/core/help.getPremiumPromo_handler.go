package core

import (
	"github.com/teamgram/proto/mtproto"
)

// HelpGetPremiumPromo
// help.getPremiumPromo#b81b93d4 = help.PremiumPromo;
func (c *PremiumCore) HelpGetPremiumPromo(in *mtproto.TLHelpGetPremiumPromo) (*mtproto.Help_PremiumPromo, error) {
	return PremiumPromo(), nil
}
