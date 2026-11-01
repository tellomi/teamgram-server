package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/internal/core"
)

// ChatEditChatCreator transfers a basic group to another member.
func (s *Service) ChatEditChatCreator(ctx context.Context, request *chat.TLChatEditChatAdmin) (*mtproto.MutableChat, error) {
	c := core.New(ctx, s.svcCtx)
	return c.ChatEditChatCreator(request)
}
