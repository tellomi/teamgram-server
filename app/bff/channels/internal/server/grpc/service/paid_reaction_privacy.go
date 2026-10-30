package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/paidprivacy"
)

// MessagesGetPaidReactionPrivacy
// messages.getPaidReactionPrivacy#472455aa = Updates;
// https://core.telegram.org/method/messages.getPaidReactionPrivacy
// https://core.telegram.org/constructor/updatePaidReactionPrivacy
// https://core.telegram.org/constructor/paidReactionPrivacyDefault
func (ReactionsService) MessagesGetPaidReactionPrivacy(context.Context, *mtproto.TLMessagesGetPaidReactionPrivacy) (*mtproto.Updates, error) {
	return paidprivacy.DefaultUpdates(), nil
}
