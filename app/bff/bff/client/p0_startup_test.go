package bff_proxy_client

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/zrpc"
)

// TestP0StartupProxyDoesNotError is the session path for methods whose gRPC
// service is not in the IDMap. Each one must return a typed object.
func TestP0StartupProxyDoesNotError(t *testing.T) {
	c := &BFFProxyClient{BFFClients: map[string]zrpc.Client{}}
	samples := []mtproto.TLObject{
		&mtproto.TLAccountCancelPasswordEmail{},
		&mtproto.TLAccountConfirmPasswordEmail{},
		&mtproto.TLAccountDeclinePasswordReset{},
		&mtproto.TLAccountGetPasswordSettings{},
		&mtproto.TLAccountGetReactionsNotifySettings{},
		&mtproto.TLAccountGetTmpPassword{Period: 30},
		&mtproto.TLAccountResendPasswordEmail{},
		&mtproto.TLAccountSetReactionsNotifySettings{},
		&mtproto.TLAccountUpdatePasswordSettings{},
		&mtproto.TLAuthFinishPasskeyLogin{},
		&mtproto.TLMessagesGetDialogFiltersEFD48C89{},
		&mtproto.TLMessagesGetDialogFiltersF19ED96D{},
		&mtproto.TLMessagesGetSuggestedDialogFilters{},
		&mtproto.TLMessagesToggleDialogFilterTags{},
		&mtproto.TLMessagesUpdateDialogFilter{},
		&mtproto.TLMessagesUpdateDialogFiltersOrder{},
		&mtproto.TLMessagesGetPaidReactionPrivacy{},
		&mtproto.TLMessagesReceivedQueue{MaxQts: 1},
	}
	for _, sample := range samples {
		got, err := c.TryReturnFakeRpcResult(context.Background(), sample)
		if err != nil || got == nil {
			t.Fatalf("%T: err=%v result=%v", sample, err, got)
		}
		if _, isErr := got.(*mtproto.TLRpcError); isErr {
			t.Fatalf("%T: rpc error", sample)
		}
		buf := mtproto.NewEncodeBuf(256)
		if err := got.Encode(buf, 232); err != nil {
			t.Fatalf("%T: %v", sample, err)
		}
		if buf.GetOffset() < 4 {
			t.Fatalf("%T: empty buffer", sample)
		}
	}

	filters, err := c.TryReturnFakeRpcResult(context.Background(), &mtproto.TLMessagesGetDialogFiltersEFD48C89{})
	if err != nil {
		t.Fatal(err)
	}
	if filters.(*mtproto.Messages_DialogFilters).GetPredicateName() != mtproto.Predicate_messages_dialogFilters {
		t.Fatal(filters.(*mtproto.Messages_DialogFilters).GetPredicateName())
	}
}
