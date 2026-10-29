package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMessagesGetPaidReactionPrivacy(t *testing.T) {
	got, err := (ReactionsService{}).MessagesGetPaidReactionPrivacy(context.Background(), &mtproto.TLMessagesGetPaidReactionPrivacy{})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_updates {
		t.Fatal(got.GetPredicateName())
	}
	if len(got.GetUpdates()) != 1 {
		t.Fatalf("updates = %d", len(got.GetUpdates()))
	}
	if got.GetUpdates()[0].GetPrivate_PAIDREACTIONPRIVACY().GetPredicateName() != mtproto.Predicate_paidReactionPrivacyDefault {
		t.Fatal(got.GetUpdates()[0].GetPredicateName())
	}
	buf := mtproto.NewEncodeBuf(128)
	if err := got.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
}
