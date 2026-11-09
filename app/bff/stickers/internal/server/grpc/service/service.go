package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
)

// sessionSync is the subset of the sync client these handlers use.
// SyncUpdatesNotMe reaches the user's other devices and leaves the
// device that made the change alone, which is what official clients expect.
type sessionSync interface {
	SyncUpdatesNotMe(ctx context.Context, in *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error)
}

// Service implements the sticker, emoji-keyword, custom-emoji, and
// message-effect RPCs.
type Service struct {
	mtproto.UnimplementedRPCStickersServer
	mtproto.UnimplementedRPCEmojiServer
	mtproto.UnimplementedRPCCustomEmojisServer
	mtproto.UnimplementedRPCMessageEffectsServer
	sync sessionSync
}

var _ mtproto.RPCGifsServer = (*Service)(nil)

func New(sync sessionSync) *Service {
	return &Service{sync: sync}
}
