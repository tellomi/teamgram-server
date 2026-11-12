package core

import (
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dao"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestMapBindAuthKeyUserErrorReturnsAuthKeyUnregistered(t *testing.T) {
	cases := []struct {
		name string
		err  error
	}{
		{"conflict", dao.ErrAuthKeyUserConflict},
		{"not-registered", dao.ErrAuthKeyNotRegistered},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := mapBindAuthKeyUserError(tc.err)
			if !errors.Is(got, mtproto.ErrAuthKeyUnregistered) {
				t.Fatalf("mapped %v", got)
			}
			rpc := mtproto.NewRpcError(got)
			if rpc.GetErrorCode() != 401 || rpc.GetErrorMessage() != "AUTH_KEY_UNREGISTERED" {
				t.Fatalf("client would see code=%d message=%s", rpc.GetErrorCode(), rpc.GetErrorMessage())
			}
		})
	}
}

func TestUnmappedBindConflictIsInternal500(t *testing.T) {
	// What the client would get if the handler returned the bare dao error
	// (or the gRPC Unknown wrapper of that error) without mapping.
	rpc := mtproto.NewRpcError(dao.ErrAuthKeyUserConflict)
	if rpc.GetErrorCode() != 500 || rpc.GetErrorMessage() != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("bare dao error: code=%d message=%s", rpc.GetErrorCode(), rpc.GetErrorMessage())
	}
	unknown := status.Error(codes.Unknown, dao.ErrAuthKeyUserConflict.Error())
	rpc = mtproto.NewRpcError(unknown)
	if rpc.GetErrorCode() != 500 {
		t.Fatalf("gRPC Unknown: code=%d message=%s", rpc.GetErrorCode(), rpc.GetErrorMessage())
	}
}

func TestMapBindAuthKeyUserErrorLeavesOtherErrors(t *testing.T) {
	in := errors.New("mysql connection refused")
	if got := mapBindAuthKeyUserError(in); got != in {
		t.Fatalf("got %v", got)
	}
	if got := mapBindAuthKeyUserError(nil); got != nil {
		t.Fatalf("nil: %v", got)
	}
}
