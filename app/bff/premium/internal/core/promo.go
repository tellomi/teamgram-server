package core

import "github.com/teamgram/proto/mtproto"

const premiumStatusText = "Tellomi includes Telegram Premium for every account. There is nothing to buy."

// PremiumPromo is help.getPremiumPromo with purchase options removed.
func PremiumPromo() *mtproto.Help_PremiumPromo {
	return mtproto.MakeTLHelpPremiumPromo(&mtproto.Help_PremiumPromo{
		StatusText:     premiumStatusText,
		StatusEntities: []*mtproto.MessageEntity{},
		VideoSections:  []string{},
		Videos:         []*mtproto.Document{},
		PeriodOptions:  []*mtproto.PremiumSubscriptionOption{},
		Users:          []*mtproto.User{},
		Currency:       "USD",
		MonthlyAmount:  0,
	}).To_Help_PremiumPromo()
}
