package chat_client

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
)

// EditChatCreator calls chat.editChatCreator. The generated client does not
// list the method; the chat service registers it beside chat.editChatAdmin.
func EditChatCreator(ctx context.Context, cli ChatClient, in *chat.TLChatEditChatAdmin) (*mtproto.MutableChat, error) {
	m, ok := cli.(*defaultChatClient)
	if !ok || m == nil || m.cli == nil || in == nil {
		return nil, mtproto.ErrInternalServerError
	}
	if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
		ctx, _ = metadata.RpcMetadataToOutgoing(ctx, md)
	}
	out := new(mtproto.MutableChat)
	if err := m.cli.Conn().Invoke(ctx, chat.ChatEditCreatorFullMethod, in, out); err != nil {
		return nil, err
	}
	return out, nil
}
