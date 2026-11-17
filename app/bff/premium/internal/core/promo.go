package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/usercopy"
)

// PremiumPromo is help.getPremiumPromo with purchase options removed.
// lang is the client language. Empty is English.
func PremiumPromo(lang string) *mtproto.Help_PremiumPromo {
	return mtproto.MakeTLHelpPremiumPromo(&mtproto.Help_PremiumPromo{
		StatusText:     usercopy.Premium(lang),
		StatusEntities: []*mtproto.MessageEntity{},
		VideoSections:  []string{},
		Videos:         []*mtproto.Document{},
		PeriodOptions:  []*mtproto.PremiumSubscriptionOption{},
		Users:          []*mtproto.User{},
		Currency:       "USD",
		MonthlyAmount:  0,
	}).To_Help_PremiumPromo()
}
