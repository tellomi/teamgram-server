package chat

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc"
)

// ChatEditCreatorFullMethod is the gRPC path for basic-group ownership transfer.
const ChatEditCreatorFullMethod = "/chat.RPCChat/chat_editChatCreator"

type chatCreatorServer interface {
	ChatEditChatCreator(context.Context, *TLChatEditChatAdmin) (*mtproto.MutableChat, error)
}

func init() {
	RPCChat_ServiceDesc.Methods = append(RPCChat_ServiceDesc.Methods, grpc.MethodDesc{
		MethodName: "chat_editChatCreator",
		Handler:    chatEditChatCreatorHandler,
	})
}

func chatEditChatCreatorHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TLChatEditChatAdmin)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(chatCreatorServer).ChatEditChatCreator(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: ChatEditCreatorFullMethod}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(chatCreatorServer).ChatEditChatCreator(ctx, req.(*TLChatEditChatAdmin))
	}
	return interceptor(ctx, in, info, handler)
}
