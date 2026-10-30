package urlauth

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestURLAuth(t *testing.T) {
	def, err := Request(nil, nil, nil, "https://example.com/login", "")
	if err != nil || def.GetPredicateName() != mtproto.Predicate_urlAuthResultDefault {
		t.Fatalf("request %+v %v", def, err)
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := def.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}

	acc, err := Accept(nil, nil, nil, "https://example.com/login", "")
	if err != nil || acc.GetPredicateName() != mtproto.Predicate_urlAuthResultAccepted || acc.GetUrl_FLAGSTRING().GetValue() != "https://example.com/login" {
		t.Fatalf("accept %+v %v", acc, err)
	}
	if err := acc.Encode(mtproto.NewEncodeBuf(256), mtproto.Layer); err != nil {
		t.Fatal(err)
	}

	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 3}).To_InputPeer()
	button, err := Request(peer, wrapperspb.Int32(4), wrapperspb.Int32(0), "", "https://web.telegram.org")
	if err != nil || button.GetPredicateName() != mtproto.Predicate_urlAuthResultDefault {
		t.Fatalf("button %+v %v", button, err)
	}

	if _, err := Request(nil, nil, nil, "not a url", ""); status.Convert(err).Message() != "URL_INVALID" {
		t.Fatalf("bad url: %v", err)
	}
	if _, err := Request(nil, nil, nil, "", "ftp://files.example"); status.Convert(err).Message() != "URL_INVALID" {
		t.Fatalf("origin: %v", err)
	}
	if err := Decline("tg://login?token=abc"); err != nil {
		t.Fatal(err)
	}
	if err := Decline(""); status.Convert(err).Message() != "URL_INVALID" {
		t.Fatalf("decline: %v", err)
	}
	ok, err := Match("https://example.com/oauth", "apple")
	if err != nil || ok {
		t.Fatalf("match %v %v", ok, err)
	}
	web := WebAuthorizations()
	if web.GetPredicateName() != mtproto.Predicate_account_webAuthorizations || len(web.GetAuthorizations()) != 0 {
		t.Fatalf("web %+v", web)
	}
	if err := ResetWebAuthorization(9); status.Convert(err).Message() != "HASH_INVALID" {
		t.Fatalf("reset: %v", err)
	}
}
