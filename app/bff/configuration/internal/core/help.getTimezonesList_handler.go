package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/configuration/internal/timezones"
)

// HelpGetTimezonesList
// help.getTimezonesList#49b30240 hash:int = help.TimezonesList;
func (c *ConfigurationCore) HelpGetTimezonesList(in *mtproto.TLHelpGetTimezonesList) (*mtproto.Help_TimezonesList, error) {
	var hash int32
	if in != nil {
		hash = in.GetHash()
	}
	return timezones.List(hash), nil
}
