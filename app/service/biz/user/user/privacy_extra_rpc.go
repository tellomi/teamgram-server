package user

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	grpc "google.golang.org/grpc"
)

const (
	RPCUser_UserGetBlockedPage_FullMethodName   = "/user.RPCUser/user_getBlockedPage"
	RPCUser_UserGetFallbackPhoto_FullMethodName = "/user.RPCUser/user_getFallbackPhoto"
)

// PrivacyExtraServer is implemented by the user service next to RPCUserServer.
// These methods stay off the generated scheme: a blocked-list page that carries
// the total count, and the public fallback profile photo.
type PrivacyExtraServer interface {
	UserGetBlockedPage(context.Context, *TLUserGetBlockedList) (*mtproto.Contacts_Blocked, error)
	UserGetFallbackPhoto(context.Context, *TLUserGetProfilePhotos) (*mtproto.Photo, error)
}

func init() {
	RPCUser_ServiceDesc.Methods = append(RPCUser_ServiceDesc.Methods,
		grpc.MethodDesc{MethodName: "user_getBlockedPage", Handler: blockedPageHandler},
		grpc.MethodDesc{MethodName: "user_getFallbackPhoto", Handler: fallbackPhotoHandler},
	)
}

func blockedPageHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TLUserGetBlockedList)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PrivacyExtraServer).UserGetBlockedPage(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: RPCUser_UserGetBlockedPage_FullMethodName}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PrivacyExtraServer).UserGetBlockedPage(ctx, req.(*TLUserGetBlockedList))
	}
	return interceptor(ctx, in, info, handler)
}

func fallbackPhotoHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TLUserGetProfilePhotos)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PrivacyExtraServer).UserGetFallbackPhoto(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: RPCUser_UserGetFallbackPhoto_FullMethodName}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PrivacyExtraServer).UserGetFallbackPhoto(ctx, req.(*TLUserGetProfilePhotos))
	}
	return interceptor(ctx, in, info, handler)
}
