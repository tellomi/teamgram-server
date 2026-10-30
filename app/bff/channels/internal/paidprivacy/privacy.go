// Package paidprivacy builds the default paid-reaction privacy update.
//
// https://core.telegram.org/method/messages.getPaidReactionPrivacy
// https://core.telegram.org/constructor/updatePaidReactionPrivacy
// https://core.telegram.org/api/reactions#paid-reaction-privacy
package paidprivacy

import "github.com/teamgram/proto/mtproto"

// DefaultUpdates is messages.getPaidReactionPrivacy's result when the account
// has not chosen a custom privacy. The default constructor shows the sender.
func DefaultUpdates() *mtproto.Updates {
	privacy := mtproto.MakeTLPaidReactionPrivacyDefault(nil).To_PaidReactionPrivacy()
	upd := mtproto.MakeTLUpdatePaidReactionPrivacy(&mtproto.Update{
		Private_PAIDREACTIONPRIVACY: privacy,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(upd)
}
