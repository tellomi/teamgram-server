package service

import "github.com/teamgram/proto/mtproto"

// Service implements the sticker, emoji-keyword, and custom-emoji RPCs.
type Service struct {
	mtproto.UnimplementedRPCStickersServer
	mtproto.UnimplementedRPCEmojiServer
	mtproto.UnimplementedRPCCustomEmojisServer
}

func New() *Service {
	return &Service{}
}
