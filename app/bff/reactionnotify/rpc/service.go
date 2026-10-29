package rpc

import (
	"context"
	"errors"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/reactionnotify"
)

// Service stores reaction notification settings for the signed-in user.
type Service struct {
	mtproto.UnimplementedRPCReactionNotificationServer
}

func New() *Service { return &Service{} }

func userID(ctx context.Context) int64 {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0
	}
	return md.GetUserId()
}

func defaultSettings() (*mtproto.ReactionsNotifySettings, error) {
	res, ok := mtproto.StartupRPCResult(&mtproto.TLAccountGetReactionsNotifySettings{})
	if !ok {
		return nil, mtproto.ErrMethodNotImpl
	}
	settings, _ := res.(*mtproto.ReactionsNotifySettings)
	if settings == nil {
		return nil, mtproto.ErrMethodNotImpl
	}
	return settings, nil
}

func (Service) AccountGetReactionsNotifySettings(ctx context.Context, _ *mtproto.TLAccountGetReactionsNotifySettings) (*mtproto.ReactionsNotifySettings, error) {
	uid := userID(ctx)
	if uid <= 0 {
		return defaultSettings()
	}
	got, err := reactionnotify.Current().Get(ctx, uid)
	if errors.Is(err, reactionnotify.ErrNotFound) {
		return defaultSettings()
	}
	if err != nil {
		return nil, err
	}
	return got, nil
}

func (Service) AccountSetReactionsNotifySettings(ctx context.Context, in *mtproto.TLAccountSetReactionsNotifySettings) (*mtproto.ReactionsNotifySettings, error) {
	uid := userID(ctx)
	if uid <= 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	settings := in.GetSettings()
	if err := reactionnotify.Current().Put(ctx, uid, settings); err != nil {
		return nil, err
	}
	got, err := reactionnotify.Current().Get(ctx, uid)
	if err != nil {
		return nil, err
	}
	return got, nil
}
