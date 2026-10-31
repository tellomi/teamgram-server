package service

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/smsjobs/internal/book"
	"github.com/teamgram/teamgram-server/app/bff/smsjobs/internal/svc"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func withUser(userID int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: userID + 10}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func msg(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func TestClosedHandlers(t *testing.T) {
	svc := New(&svc.ServiceContext{Book: book.New()})
	ctx := withUser(4)
	if _, err := svc.SmsjobsIsEligibleToJoin(context.Background(), nil); msg(err) != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
	if c := status.Convert(mustErr(t, func() error {
		_, err := svc.SmsjobsIsEligibleToJoin(ctx, &mtproto.TLSmsjobsIsEligibleToJoin{})
		return err
	})); c.Message() != "NOT_ELIGIBLE" || int(c.Code()) != int(mtproto.ErrForbidden) {
		t.Fatalf("eligible %d %s", c.Code(), c.Message())
	}
	if c := status.Convert(mustErr(t, func() error {
		_, err := svc.SmsjobsJoin(ctx, &mtproto.TLSmsjobsJoin{})
		return err
	})); c.Message() != "NOT_ELIGIBLE" || int(c.Code()) != int(mtproto.ErrBadRequest) {
		t.Fatalf("join %d %s", c.Code(), c.Message())
	}
	if _, err := svc.SmsjobsLeave(ctx, &mtproto.TLSmsjobsLeave{}); msg(err) != "NOT_JOINED" {
		t.Fatal(err)
	}
	if _, err := svc.SmsjobsUpdateSettings(ctx, &mtproto.TLSmsjobsUpdateSettings{AllowInternational: true}); msg(err) != "NOT_JOINED" {
		t.Fatal(err)
	}
	if _, err := svc.SmsjobsGetStatus(ctx, &mtproto.TLSmsjobsGetStatus{}); msg(err) != "NOT_JOINED" {
		t.Fatal(err)
	}
	if _, err := svc.SmsjobsGetSmsJob(ctx, &mtproto.TLSmsjobsGetSmsJob{JobId: "x"}); msg(err) != "SMSJOB_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.SmsjobsFinishJob(ctx, &mtproto.TLSmsjobsFinishJob{JobId: "x", Error: wrapperspb.String("PHONE_NOT_FOUND")}); msg(err) != "SMSJOB_ID_INVALID" {
		t.Fatal(err)
	}
}

func mustErr(t *testing.T, fn func() error) error {
	t.Helper()
	err := fn()
	if err == nil {
		t.Fatal("expected error")
	}
	return err
}
