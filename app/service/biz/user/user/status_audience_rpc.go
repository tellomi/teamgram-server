package user

import (
	"context"

	grpc "google.golang.org/grpc"
)

const (
	RPCUser_UserGetReverseContactIdList_FullMethodName = "/user.RPCUser/user_getReverseContactIdList"
	RPCUser_UserBlockedEitherWay_FullMethodName        = "/user.RPCUser/user_blockedEitherWay"
)

// StatusAudienceServer is implemented by the user service next to RPCUserServer.
// These methods stay off the generated interface so a privacy fanout can load
// reverse-contact ids and a block set without a scheme regeneration.
type StatusAudienceServer interface {
	UserGetReverseContactIdList(context.Context, *TLUserGetContactIdList) (*Vector_Long, error)
	UserBlockedEitherWay(context.Context, *TLUserCheckBlockUserList) (*Vector_Long, error)
}

func init() {
	RPCUser_ServiceDesc.Methods = append(RPCUser_ServiceDesc.Methods,
		grpc.MethodDesc{MethodName: "user_getReverseContactIdList", Handler: statusReverseContactHandler},
		grpc.MethodDesc{MethodName: "user_blockedEitherWay", Handler: statusBlockedEitherHandler},
	)
}

func statusReverseContactHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TLUserGetContactIdList)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StatusAudienceServer).UserGetReverseContactIdList(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: RPCUser_UserGetReverseContactIdList_FullMethodName}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StatusAudienceServer).UserGetReverseContactIdList(ctx, req.(*TLUserGetContactIdList))
	}
	return interceptor(ctx, in, info, handler)
}

func statusBlockedEitherHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TLUserCheckBlockUserList)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(StatusAudienceServer).UserBlockedEitherWay(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: RPCUser_UserBlockedEitherWay_FullMethodName}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(StatusAudienceServer).UserBlockedEitherWay(ctx, req.(*TLUserCheckBlockUserList))
	}
	return interceptor(ctx, in, info, handler)
}
