package service

import "github.com/teamgram/proto/mtproto"

// Service implements the sticker, emoji-keyword, custom-emoji, and
// message-effect RPCs.
type Service struct {
	mtproto.UnimplementedRPCStickersServer
	mtproto.UnimplementedRPCEmojiServer
	mtproto.UnimplementedRPCCustomEmojisServer
	mtproto.UnimplementedRPCMessageEffectsServer
}

var _ mtproto.RPCGifsServer = (*Service)(nil)

func New() *Service {
	return &Service{}
}
