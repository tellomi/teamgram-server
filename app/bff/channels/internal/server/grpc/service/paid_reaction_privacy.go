package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/paidprivacy"
	"github.com/teamgram/teamgram-server/app/bff/channels/internal/paidreaction"
)

// MessagesGetPaidReactionPrivacy
// messages.getPaidReactionPrivacy#472455aa = Updates;
// https://core.telegram.org/method/messages.getPaidReactionPrivacy
// https://core.telegram.org/constructor/updatePaidReactionPrivacy
// https://core.telegram.org/constructor/paidReactionPrivacyDefault
func (ReactionsService) MessagesGetPaidReactionPrivacy(context.Context, *mtproto.TLMessagesGetPaidReactionPrivacy) (*mtproto.Updates, error) {
	return paidprivacy.DefaultUpdates(), nil
}

// MessagesSendPaidReaction
// messages.sendPaidReaction#58bbcb50 flags:# peer:InputPeer msg_id:int count:int random_id:long private:flags.0?PaidReactionPrivacy = Updates;
func (ReactionsService) MessagesSendPaidReaction(ctx context.Context, in *mtproto.TLMessagesSendPaidReaction) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	return paidreaction.Send(self, in.GetPeer(), in.GetMsgId(), in.GetCount(), in.GetRandomId(), in.GetPrivate_FLAGPAIDREACTIONPRIVACY())
}

// MessagesTogglePaidReactionPrivacy
// messages.togglePaidReactionPrivacy#435885b5 peer:InputPeer msg_id:int private:PaidReactionPrivacy = Bool;
func (ReactionsService) MessagesTogglePaidReactionPrivacy(ctx context.Context, in *mtproto.TLMessagesTogglePaidReactionPrivacy) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if err := paidreaction.Toggle(self, in.GetPeer(), in.GetMsgId(), in.GetPrivate_PAIDREACTIONPRIVACY()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
