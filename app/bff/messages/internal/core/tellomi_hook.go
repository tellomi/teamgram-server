package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/enforcement"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
	"github.com/zeromicro/go-zero/core/contextx"
	"github.com/zeromicro/go-zero/core/threading"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// maybeTellomi replies as @tellomi after the user's private message is stored.
// It does not run for scheduled messages or the service account's own sends.
// Groups are handled by noteTellomi. A switched-off assistant still replies
// with the fixed not-open sentence when a service is installed.
func maybeTellomi(ctx context.Context, from int64, peer *mtproto.PeerUtil, text string, random int64, replyTo int32) {
	if !tellomiPrivate(from, peer) {
		return
	}
	if !tellomiai.Installed() && !enforcement.AdminReplyOn() {
		return
	}
	ctx = contextx.ValueOnlyFrom(ctx)
	threading.GoSafe(func() {
		if enforcement.ObserveAdmin(ctx, from, text, replyTo) {
			return
		}
		if tellomiai.Installed() {
			tellomiai.ObservePrivate(ctx, from, text, random)
		}
	})
}

func replyToMsgID(in *mtproto.TLMessagesSendMessage) int32 {
	if in == nil {
		return 0
	}
	return replyTarget(in.GetReplyToMsgId(), in.GetReplyTo())
}

func replyTarget(flag *wrapperspb.Int32Value, reply *mtproto.InputReplyTo) int32 {
	if flag != nil && flag.GetValue() != 0 {
		return flag.GetValue()
	}
	if reply != nil {
		return reply.GetReplyToMsgId()
	}
	return 0
}

func tellomiPrivate(from int64, peer *mtproto.PeerUtil) bool {
	if from == 0 || from == tellomiai.AccountID || peer == nil || !peer.IsUser() {
		return false
	}
	return peer.PeerId == tellomiai.AccountID
}
