package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/enforcement"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
	"github.com/zeromicro/go-zero/core/contextx"
	"github.com/zeromicro/go-zero/core/threading"
)

// maybeTellomi replies as @tellomi after the user's private message is stored.
// The feature flag is off unless TELLOMI_AI=1, and then this returns before
// starting a goroutine. It does not run for scheduled messages, groups, or
// the service account's own sends.
func maybeTellomi(ctx context.Context, from int64, peer *mtproto.PeerUtil, text string, random int64, replyTo int32) {
	if !tellomiPrivate(from, peer) {
		return
	}
	if !tellomiai.Enabled() && !enforcement.AdminReplyOn() {
		return
	}
	ctx = contextx.ValueOnlyFrom(ctx)
	threading.GoSafe(func() {
		if enforcement.ObserveAdmin(ctx, from, text, replyTo) {
			return
		}
		if tellomiai.Enabled() {
			tellomiai.ObservePrivate(ctx, from, text, random)
		}
	})
}

func replyToMsgID(in *mtproto.TLMessagesSendMessage) int32 {
	if in == nil {
		return 0
	}
	if v := in.GetReplyToMsgId(); v != nil && v.GetValue() != 0 {
		return v.GetValue()
	}
	if in.GetReplyTo() != nil {
		return in.GetReplyTo().GetReplyToMsgId()
	}
	return 0
}

func tellomiPrivate(from int64, peer *mtproto.PeerUtil) bool {
	if from == 0 || from == tellomiai.AccountID || peer == nil || !peer.IsUser() {
		return false
	}
	return peer.PeerId == tellomiai.AccountID
}
