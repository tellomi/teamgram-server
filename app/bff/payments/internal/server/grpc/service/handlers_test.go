package service

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/payments/internal/book"
	"github.com/teamgram/teamgram-server/app/bff/payments/internal/svc"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func withUser(userID int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: userID + 10}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func TestStarsRequireAuth(t *testing.T) {
	svc := New(&svc.ServiceContext{Book: book.New()})
	if _, err := svc.PaymentsGetStarsStatus(context.Background(), &mtproto.TLPaymentsGetStarsStatus{}); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
	peer := mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
	got, err := svc.PaymentsGetStarsStatus(withUser(4), &mtproto.TLPaymentsGetStarsStatus{Peer: peer})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetBalance_STARSAMOUNT().GetPredicateName() != mtproto.Predicate_starsAmount {
		t.Fatal(got.GetBalance_STARSAMOUNT())
	}
}
