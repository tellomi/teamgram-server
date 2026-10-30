package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestMessageStatsAndGraph(t *testing.T) {
	svc, ctx := newTest(t)
	up, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	in := inCh(ch)
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{
		RandomID: 1,
		Message:  &mtproto.Message{Message: "hello", Date: 50, Views: wrapperspb.Int32(3), Forwards: wrapperspb.Int32(2)},
	}})
	if err != nil || sent == nil {
		t.Fatal(err)
	}
	stats, err := svc.MessageStats(ctx, 1, in, 1)
	if err != nil || stats.GetViewsGraph().GetToken() == "" || stats.GetReactionsByEmotionGraph().GetToken() == "" {
		t.Fatalf("stats %#v %v", stats, err)
	}
	graph, err := ReadGraph(stats.GetViewsGraph().GetToken())
	if err != nil || graph.GetJson().GetData() == "" {
		t.Fatalf("graph %#v %v", graph, err)
	}
	if _, err := ReadGraph("nope"); status.Convert(err).Message() != "GRAPH_INVALID_RELOAD" {
		t.Fatal(err)
	}
	if _, err := ReadGraph("expired"); status.Convert(err).Message() != "GRAPH_EXPIRED_RELOAD" {
		t.Fatal(err)
	}
	if _, err := svc.MessageStats(ctx, 2, in, 1); err == nil {
		t.Fatal("non-member read stats")
	}
	fwd, err := svc.MessagePublicForwards(ctx, 1, in, 1, "", 20)
	if err != nil || len(fwd.GetForwards()) != 0 {
		t.Fatalf("forwards %#v %v", fwd, err)
	}
	if _, err := svc.MessagePublicForwards(ctx, 1, in, 1, "later", 20); status.Convert(err).Message() != "OFFSET_INVALID" {
		t.Fatal(err)
	}
	if _, err := svc.MessageStats(ctx, 1, in, 0); status.Convert(err).Message() != "MESSAGE_ID_INVALID" {
		t.Fatal(err)
	}
	zero, err := ReadGraph("0")
	if err != nil || zero.GetPredicateName() != mtproto.Predicate_statsGraph {
		t.Fatalf("token 0 %#v %v", zero, err)
	}
}
