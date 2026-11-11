package e2e

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/gotd/td/tgerr"
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

func TestDecodeLangPackVector(t *testing.T) {
	hi := mtproto.MakeTLLangPackLanguage(&mtproto.LangPackLanguage{
		Official: true, Name: "Hindi", NativeName: "हिन्दी", LangCode: "hi",
		PluralCode: "hi", StringsCount: 10, TranslatedCount: 2, TranslationsUrl: "https://tellomi.cn/hi",
	})
	fr := mtproto.MakeTLLangPackLanguage(&mtproto.LangPackLanguage{
		Official: true, Name: "French", NativeName: "Français", LangCode: "fr",
		PluralCode: "fr", StringsCount: 10, TranslatedCount: 10, TranslationsUrl: "https://tellomi.cn/fr",
	})
	buf := mtproto.NewEncodeBuf(512)
	buf.UInt(crcVector)
	buf.Int(2)
	if err := hi.Encode(buf, layer); err != nil {
		t.Fatal(err)
	}
	if err := fr.Encode(buf, layer); err != nil {
		t.Fatal(err)
	}
	obj, err := decodeTL(buf.GetBuf())
	if err != nil {
		t.Fatal(err)
	}
	vec, ok := obj.(*tlVector)
	if !ok || vec.Count != 2 || len(vec.Langs) != 2 || vec.Langs[0].GetLangCode() != "hi" || vec.Langs[1].GetLangCode() != "fr" {
		t.Fatalf("%T %+v", obj, vec)
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

func TestFloodWait(t *testing.T) {
	wait, ok := floodWait(&tgerr.Error{Code: 420, Message: "FLOOD_WAIT (7)"})
	if !ok || wait != 8*time.Second {
		t.Fatalf("paren wait %s %v", wait, ok)
	}
	wait, ok = floodWait(&tgerr.Error{Code: 420, Message: "FLOOD_WAIT_3"})
	if !ok || wait != 4*time.Second {
		t.Fatalf("underscore wait %s %v", wait, ok)
	}
	if _, ok = floodWait(&tgerr.Error{Code: 400, Message: "FLOOD_WAIT (1)"}); ok {
		t.Fatal("non-420 accepted")
	}
}

func TestDeadlockIsRetried(t *testing.T) {
	wait, ok := retryWait(&tgerr.Error{Code: 500, Message: "Error 1213 (40001): Deadlock found when trying to get lock; try restarting transaction"})
	if !ok || wait != 200*time.Millisecond {
		t.Fatalf("deadlock wait %s %v", wait, ok)
	}
	if _, ok = retryWait(&tgerr.Error{Code: 500, Message: "INTERNAL_SERVER_ERROR"}); ok {
		t.Fatal("unrelated 500 was retried")
	}
	if _, ok = retryWait(&tgerr.Error{Code: 400, Message: "Deadlock found"}); ok {
		t.Fatal("non-500 deadlock was retried")
	}
}
