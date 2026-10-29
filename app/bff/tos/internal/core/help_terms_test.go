package core

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/tos/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/tos/terms"
)

func TestHandlersAcceptThenEmpty(t *testing.T) {
	svcCtx := &svc.ServiceContext{Terms: terms.New(terms.Config{UpdateId: "tellomi-tos-2", PopupEnabled: true}, nil)}
	c := New(context.Background(), svcCtx)
	c.MD = &metadata.RpcMetadata{UserId: 42}

	first, err := c.HelpGetTermsOfServiceUpdate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if first.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdate {
		t.Fatalf("predicate %s", first.GetPredicateName())
	}
	id := first.GetTermsOfService().GetId()
	ok, err := c.HelpAcceptTermsOfService(&mtproto.TLHelpAcceptTermsOfService{Id: id})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatalf("accept ok=%v err=%v", ok, err)
	}
	second, err := c.HelpGetTermsOfServiceUpdate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if second.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdateEmpty {
		t.Fatalf("predicate %s", second.GetPredicateName())
	}
	now := int32(time.Now().Unix())
	if second.GetExpires() < now+terms.EmptyTTL-5 || second.GetExpires() > now+terms.EmptyTTL+5 {
		t.Fatalf("expires %d now %d", second.GetExpires(), now)
	}
}

func TestDefaultHandlerIsEmpty(t *testing.T) {
	c := New(context.Background(), &svc.ServiceContext{Terms: terms.New(terms.Config{}, nil)})
	c.MD = &metadata.RpcMetadata{UserId: 1}
	got, err := c.HelpGetTermsOfServiceUpdate(nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdateEmpty {
		t.Fatalf("predicate %s", got.GetPredicateName())
	}
}
