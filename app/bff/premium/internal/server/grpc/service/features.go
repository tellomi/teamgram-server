package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/premium/internal/core"
)

func (s *Service) AccountUpdateEmojiStatus(ctx context.Context, request *mtproto.TLAccountUpdateEmojiStatus) (*mtproto.Bool, error) {
	return core.New(ctx, s.svcCtx).AccountUpdateEmojiStatus(request)
}

func (s *Service) AccountGetDefaultEmojiStatuses(ctx context.Context, request *mtproto.TLAccountGetDefaultEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	return core.New(ctx, s.svcCtx).AccountGetDefaultEmojiStatuses(request)
}

func (s *Service) AccountGetRecentEmojiStatuses(ctx context.Context, request *mtproto.TLAccountGetRecentEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	return core.New(ctx, s.svcCtx).AccountGetRecentEmojiStatuses(request)
}

func (s *Service) AccountClearRecentEmojiStatuses(ctx context.Context, request *mtproto.TLAccountClearRecentEmojiStatuses) (*mtproto.Bool, error) {
	return core.New(ctx, s.svcCtx).AccountClearRecentEmojiStatuses(request)
}

func (s *Service) AccountGetChannelDefaultEmojiStatuses(ctx context.Context, request *mtproto.TLAccountGetChannelDefaultEmojiStatuses) (*mtproto.Account_EmojiStatuses, error) {
	return core.New(ctx, s.svcCtx).AccountGetChannelDefaultEmojiStatuses(request)
}

func (s *Service) AccountUpdateColor(ctx context.Context, request *mtproto.TLAccountUpdateColor) (*mtproto.Bool, error) {
	return core.New(ctx, s.svcCtx).AccountUpdateColor(request)
}

func (s *Service) ChannelsUpdateColor(ctx context.Context, request *mtproto.TLChannelsUpdateColor) (*mtproto.Updates, error) {
	return core.New(ctx, s.svcCtx).ChannelsUpdateColor(request)
}

func (s *Service) ChannelsUpdateEmojiStatus(ctx context.Context, request *mtproto.TLChannelsUpdateEmojiStatus) (*mtproto.Updates, error) {
	return core.New(ctx, s.svcCtx).ChannelsUpdateEmojiStatus(request)
}

func (s *Service) AccountGetDefaultBackgroundEmojis(ctx context.Context, request *mtproto.TLAccountGetDefaultBackgroundEmojis) (*mtproto.EmojiList, error) {
	return core.New(ctx, s.svcCtx).AccountGetDefaultBackgroundEmojis(request)
}

func (s *Service) HelpGetPeerColors(ctx context.Context, request *mtproto.TLHelpGetPeerColors) (*mtproto.Help_PeerColors, error) {
	return core.New(ctx, s.svcCtx).HelpGetPeerColors(request)
}

func (s *Service) HelpGetPeerProfileColors(ctx context.Context, request *mtproto.TLHelpGetPeerProfileColors) (*mtproto.Help_PeerColors, error) {
	return core.New(ctx, s.svcCtx).HelpGetPeerProfileColors(request)
}

func (s *Service) MessagesTranscribeAudio(ctx context.Context, request *mtproto.TLMessagesTranscribeAudio) (*mtproto.Messages_TranscribedAudio, error) {
	return core.New(ctx, s.svcCtx).MessagesTranscribeAudio(request)
}

func (s *Service) MessagesRateTranscribedAudio(ctx context.Context, request *mtproto.TLMessagesRateTranscribedAudio) (*mtproto.Bool, error) {
	return core.New(ctx, s.svcCtx).MessagesRateTranscribedAudio(request)
}
