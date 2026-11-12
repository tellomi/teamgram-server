package userclient

import (
	"context"

	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc"
)

func (m *defaultUserClient) UserGetReverseContactIdList(ctx context.Context, in *user.TLUserGetContactIdList) (*user.Vector_Long, error) {
	if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
		ctx, _ = metadata.RpcMetadataToOutgoing(ctx, md)
	}
	out := new(user.Vector_Long)
	err := m.cli.Conn().Invoke(ctx, user.RPCUser_UserGetReverseContactIdList_FullMethodName, in, out, grpc.StaticMethod())
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (m *defaultUserClient) UserBlockedEitherWay(ctx context.Context, in *user.TLUserCheckBlockUserList) (*user.Vector_Long, error) {
	if md := metadata.RpcMetadataFromIncoming(ctx); md != nil {
		ctx, _ = metadata.RpcMetadataToOutgoing(ctx, md)
	}
	out := new(user.Vector_Long)
	err := m.cli.Conn().Invoke(ctx, user.RPCUser_UserBlockedEitherWay_FullMethodName, in, out, grpc.StaticMethod())
	if err != nil {
		return nil, err
	}
	return out, nil
}
