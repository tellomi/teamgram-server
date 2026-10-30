// Package phonehistory deletes call-log service messages.
//
// https://core.telegram.org/method/messages.deletePhoneCallHistory
package phonehistory

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// Deleter is the messenger method that removes phone-call service messages.
type Deleter interface {
	MsgDeletePhoneCallHistory(ctx context.Context, in *msg.TLMsgDeletePhoneCallHistory) (*mtproto.Messages_AffectedFoundMessages, error)
}

// Delete removes the caller's phone-call history. With no messenger client it
// returns an empty affectedFoundMessages, which is the result when nothing matched.
func Delete(ctx context.Context, d Deleter, userID, authKeyID int64, revoke bool) (*mtproto.Messages_AffectedFoundMessages, error) {
	if d != nil {
		return d.MsgDeletePhoneCallHistory(ctx, &msg.TLMsgDeletePhoneCallHistory{
			UserId: userID, AuthKeyId: authKeyID, Revoke: revoke,
		})
	}
	return mtproto.MakeTLMessagesAffectedFoundMessages(&mtproto.Messages_AffectedFoundMessages{
		Messages: []int32{},
	}).To_Messages_AffectedFoundMessages(), nil
}
