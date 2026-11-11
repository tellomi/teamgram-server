package sess

import (
	"context"
	"net"
	"testing"

	"github.com/teamgram/proto/mtproto"
	sessionsvc "github.com/teamgram/teamgram-server/app/interface/session/session"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

// The expected user has to survive the same gRPC codec zrpc uses for unary
// session.pushUpdatesData and the codec StreamingSession uses for the
// bidi stream. A dropped field comes back as 0 and the session check no
// longer knows who the update was for.
func TestExpectedUserSurvivesUnaryAndStream(t *testing.T) {
	const (
		userA int64 = 11
		userB int64 = 22
		keyA  int64 = 100
		keyB  int64 = 200
	)
	unaryCh := make(chan *sessionsvc.TLSessionPushUpdatesData, 1)
	streamCh := make(chan *sessionsvc.TLSessionPushUpdatesData, 1)
	sessionCh := make(chan *sessionsvc.TLSessionPushSessionUpdatesData, 1)

	lis := bufconn.Listen(1 << 20)
	srv := grpc.NewServer()
	sessionsvc.RegisterRPCSessionServer(srv, &pushUserUnary{ch: unaryCh})
	sessionsvc.RegisterRPCSessionStreamServer(srv, &pushUserStream{updates: streamCh, sessionUpdates: sessionCh})
	go func() { _ = srv.Serve(lis) }()
	t.Cleanup(srv.Stop)

	conn, err := grpc.DialContext(context.Background(), "bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })

	unaryMsg := &sessionsvc.TLSessionPushUpdatesData{
		PermAuthKeyId: keyA,
		Notification:  true,
		Updates:       mtproto.MakeTLUpdatesTooLong(nil).To_Updates(),
	}
	sessionsvc.SetPushUpdatesUser(unaryMsg, userA)
	if _, err = sessionsvc.NewRPCSessionClient(conn).SessionPushUpdatesData(context.Background(), unaryMsg); err != nil {
		t.Fatal(err)
	}
	gotUnary := <-unaryCh
	if sessionsvc.PushUpdatesUser(gotUnary) != userA || gotUnary.GetPermAuthKeyId() != keyA {
		t.Fatalf("unary user %d key %d", sessionsvc.PushUpdatesUser(gotUnary), gotUnary.GetPermAuthKeyId())
	}

	stream, err := sessionsvc.NewRPCSessionStreamClient(conn).SessionDataStream(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	streamMsg := &sessionsvc.TLSessionPushUpdatesData{
		PermAuthKeyId: keyB,
		Updates:       mtproto.MakeTLUpdatesTooLong(nil).To_Updates(),
	}
	sessionsvc.SetPushUpdatesUser(streamMsg, userB)
	if err = stream.Send(&sessionsvc.SessionStreamRequest{
		RequestId: "1",
		Payload:   &sessionsvc.SessionStreamRequest_PushUpdates{PushUpdates: streamMsg},
	}); err != nil {
		t.Fatal(err)
	}
	gotStream := <-streamCh
	if sessionsvc.PushUpdatesUser(gotStream) != userB || gotStream.GetPermAuthKeyId() != keyB {
		t.Fatalf("stream user %d key %d", sessionsvc.PushUpdatesUser(gotStream), gotStream.GetPermAuthKeyId())
	}

	one := &sessionsvc.TLSessionPushSessionUpdatesData{
		PermAuthKeyId: keyA,
		AuthKeyId:     keyA,
		SessionId:     7,
		Updates:       mtproto.MakeTLUpdatesTooLong(nil).To_Updates(),
	}
	sessionsvc.SetPushSessionUpdatesUser(one, userA)
	if err = stream.Send(&sessionsvc.SessionStreamRequest{
		RequestId: "2",
		Payload:   &sessionsvc.SessionStreamRequest_PushSessionUpdates{PushSessionUpdates: one},
	}); err != nil {
		t.Fatal(err)
	}
	gotOne := <-sessionCh
	if sessionsvc.PushSessionUpdatesUser(gotOne) != userA || gotOne.GetSessionId() != 7 {
		t.Fatalf("session push user %d session %d", sessionsvc.PushSessionUpdatesUser(gotOne), gotOne.GetSessionId())
	}

	accountA := newOwnerWrapper(nil, userA, nil)
	accountB := newOwnerWrapper(nil, userB, nil)
	a := onlineGeneric(accountA)
	b := onlineGeneric(accountB)
	deliverSync(accountA, sessionsvc.PushUpdatesUser(gotUnary))
	deliverSync(accountB, sessionsvc.PushUpdatesUser(gotStream))
	deliverSync(accountB, sessionsvc.PushUpdatesUser(gotUnary))
	if a.outQueue.oMsgs.Len() != 1 {
		t.Fatalf("A queue %d", a.outQueue.oMsgs.Len())
	}
	if b.outQueue.oMsgs.Len() != 1 {
		t.Fatalf("B queue %d, A's update must not arrive", b.outQueue.oMsgs.Len())
	}
}

type pushUserUnary struct {
	sessionsvc.UnimplementedRPCSessionServer
	ch chan *sessionsvc.TLSessionPushUpdatesData
}

func (s *pushUserUnary) SessionPushUpdatesData(_ context.Context, in *sessionsvc.TLSessionPushUpdatesData) (*mtproto.Bool, error) {
	s.ch <- in
	return mtproto.BoolTrue, nil
}

type pushUserStream struct {
	sessionsvc.UnimplementedRPCSessionStreamServer
	updates        chan *sessionsvc.TLSessionPushUpdatesData
	sessionUpdates chan *sessionsvc.TLSessionPushSessionUpdatesData
}

func (s *pushUserStream) SessionDataStream(stream grpc.BidiStreamingServer[sessionsvc.SessionStreamRequest, sessionsvc.SessionStreamResponse]) error {
	for {
		req, err := stream.Recv()
		if err != nil {
			return err
		}
		if p := req.GetPushUpdates(); p != nil {
			s.updates <- p
		}
		if p := req.GetPushSessionUpdates(); p != nil {
			s.sessionUpdates <- p
		}
		if err = stream.Send(&sessionsvc.SessionStreamResponse{RequestId: req.GetRequestId()}); err != nil {
			return err
		}
	}
}
