package bff_proxy_client

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/zrpc"
)

func TestP0StartupProxy(t *testing.T) {
	c := &BFFProxyClient{BFFClients: map[string]zrpc.Client{}}
	reads := []mtproto.TLObject{
		&mtproto.TLAccountGetPasswordSettings{},
		&mtproto.TLAccountGetReactionsNotifySettings{},
		&mtproto.TLAccountGetTmpPassword{Period: 30},
		&mtproto.TLAuthFinishPasskeyLogin{},
		&mtproto.TLMessagesGetDialogFiltersEFD48C89{},
		&mtproto.TLMessagesGetDialogFiltersF19ED96D{},
		&mtproto.TLMessagesGetSuggestedDialogFilters{},
		&mtproto.TLMessagesGetPaidReactionPrivacy{},
		&mtproto.TLMessagesReceivedQueue{MaxQts: 1},
	}
	for _, sample := range reads {
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
	}

	writes := []struct {
		in  mtproto.TLObject
		msg string
	}{
		{&mtproto.TLAccountUpdatePasswordSettings{}, "PASSWORD_SETTINGS_UNAVAILABLE"},
		{&mtproto.TLAccountConfirmPasswordEmail{}, "CODE_INVALID"},
		{&mtproto.TLAccountResendPasswordEmail{}, "EMAIL_NOT_SETUP"},
		{&mtproto.TLAccountCancelPasswordEmail{}, "EMAIL_NOT_SETUP"},
		{&mtproto.TLAccountDeclinePasswordReset{}, "PASSWORD_RECOVERY_NA"},
	}
	for _, sample := range writes {
		got, err := c.TryReturnFakeRpcResult(context.Background(), sample.in)
		if err != nil {
			t.Fatalf("%T: %v", sample.in, err)
		}
		rpc, ok := got.(*mtproto.TLRpcError)
		if !ok || rpc.GetErrorCode() != 400 || rpc.GetErrorMessage() != sample.msg {
			t.Fatalf("%T: %#v", sample.in, got)
		}
	}

	// Folder and reaction writes are persisted by routed handlers. The
	// unauthenticated fallback must not answer them with BoolTrue.
	for _, sample := range []mtproto.TLObject{
		&mtproto.TLMessagesUpdateDialogFilter{},
		&mtproto.TLMessagesUpdateDialogFiltersOrder{},
		&mtproto.TLMessagesToggleDialogFilterTags{},
		&mtproto.TLAccountSetReactionsNotifySettings{},
	} {
		got, err := c.TryReturnFakeRpcResult(context.Background(), sample)
		if err == nil {
			t.Fatalf("%T: fallback returned %#v", sample, got)
		}
		if _, isBool := got.(*mtproto.Bool); isBool {
			t.Fatalf("%T: fake bool success", sample)
		}
	}
}
