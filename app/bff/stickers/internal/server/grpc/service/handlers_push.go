package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/zeromicro/go-zero/core/logx"
)

func permAuthKeyID(ctx context.Context) int64 {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0
	}
	return md.GetPermAuthKeyId()
}

func (s *Service) pushNotMe(ctx context.Context, updates *mtproto.Updates) {
	if s == nil || s.sync == nil || updates == nil {
		return
	}
	uid := userID(ctx)
	if uid == 0 {
		return
	}
	if _, err := s.sync.SyncUpdatesNotMe(ctx, &syncpb.TLSyncUpdatesNotMe{
		UserId:        uid,
		PermAuthKeyId: permAuthKeyID(ctx),
		Updates:       updates,
	}); err != nil {
		logx.Errorf("stickers push: %v", err)
	}
}

func (s *Service) pushSets(ctx context.Context, updates []*mtproto.Updates) {
	for _, up := range updates {
		s.pushNotMe(ctx, up)
	}
}
