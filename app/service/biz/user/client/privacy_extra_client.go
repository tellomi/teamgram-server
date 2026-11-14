package userclient

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc"
)

func (m *defaultUserClient) UserGetBlockedPage(ctx context.Context, in *user.TLUserGetBlockedList) (*mtproto.Contacts_Blocked, error) {
	if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
		ctx, _ = metadata.RpcMetadataToOutgoing(ctx, md)
	}
	out := new(mtproto.Contacts_Blocked)
	err := m.cli.Conn().Invoke(ctx, user.RPCUser_UserGetBlockedPage_FullMethodName, in, out, grpc.StaticMethod())
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (m *defaultUserClient) UserGetFallbackPhoto(ctx context.Context, in *user.TLUserGetProfilePhotos) (*mtproto.Photo, error) {
	if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
		ctx, _ = metadata.RpcMetadataToOutgoing(ctx, md)
	}
	out := new(mtproto.Photo)
	err := m.cli.Conn().Invoke(ctx, user.RPCUser_UserGetFallbackPhoto_FullMethodName, in, out, grpc.StaticMethod())
	if err != nil {
		return nil, err
	}
	return out, nil
}
