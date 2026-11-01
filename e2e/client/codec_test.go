package e2e

import (
	"encoding/binary"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestTestPhoneCode(t *testing.T) {
	if got := TestPhoneCode("9996610001"); got != "11111" {
		t.Fatalf("code = %s", got)
	}
	if got := TestPhoneCode("+99966 2 0001"); got != "22222" {
		t.Fatalf("code = %s", got)
	}
}

func TestInvokeWithLayerEncodes(t *testing.T) {
	cfg := Config{APIID: 10840, APIHash: "0123456789abcdef0123456789abcdef", DC: 1}
	inner, err := encodeTL(&mtproto.TLHelpGetConfig{Constructor: mtproto.CRC32_help_getConfig})
	if err != nil {
		t.Fatal(err)
	}
	if int32(binary.LittleEndian.Uint32(inner)) != int32(mtproto.CRC32_help_getConfig) {
		t.Fatalf("help.getConfig ctor 0x%x", binary.LittleEndian.Uint32(inner))
	}
	wrapped, err := wrapInit(cfg, ProfileIOS, inner)
	if err != nil {
		t.Fatal(err)
	}
	if int32(binary.LittleEndian.Uint32(wrapped)) != int32(mtproto.CRC32_invokeWithLayer) {
		t.Fatalf("outer ctor 0x%x", binary.LittleEndian.Uint32(wrapped))
	}
	obj, err := decodeTL(wrapped)
	if err != nil {
		t.Fatal(err)
	}
	inv, ok := obj.(*mtproto.TLInvokeWithLayer)
	if !ok || inv.GetLayer() != 232 || len(inv.GetQuery()) < 4 {
		t.Fatalf("decoded %#v", obj)
	}
}

func TestServerKeyFingerprint(t *testing.T) {
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	key, err := cfg.PublicKey()
	if err != nil {
		t.Fatal(err)
	}
	const want uint64 = 12240908862933197005
	if got := uint64(key.Fingerprint()); got != want {
		t.Fatalf("fingerprint %d, want %d", got, want)
	}
}
