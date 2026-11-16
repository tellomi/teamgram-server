package e2e

import (
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestAccountCatalogs checks background and restricted emoji lists, a wav
// ringtone upload, and the passkey relying-party id.
func TestAccountCatalogs(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()
	a, _ := dialPhone(t, ctx, cfg, "9996610774", "CatalogA")

	bg, err := a.Invoke(ctx, &mtproto.TLAccountGetDefaultBackgroundEmojis{
		Constructor: mtproto.CRC32_account_getDefaultBackgroundEmojis,
	})
	if err != nil {
		t.Fatal(err)
	}
	list := emojiListOf(t, bg)
	if len(list.GetDocumentId()) < 8 {
		t.Fatalf("background len %d", len(list.GetDocumentId()))
	}
	restricted, err := a.Invoke(ctx, &mtproto.TLAccountGetChannelRestrictedStatusEmojis{
		Constructor: mtproto.CRC32_account_getChannelRestrictedStatusEmojis,
	})
	if err != nil {
		t.Fatal(err)
	}
	rlist := emojiListOf(t, restricted)
	if len(rlist.GetDocumentId()) < 8 || rlist.GetHash() == list.GetHash() {
		t.Fatalf("restricted len %d", len(rlist.GetDocumentId()))
	}

	doc, err := a.Invoke(ctx, &mtproto.TLAccountUploadRingtone{
		Constructor: mtproto.CRC32_account_uploadRingtone,
		File: mtproto.MakeTLInputFile(&mtproto.InputFile{
			Id_INT64: randomID(), Parts: 1, Name: "ping.wav",
		}).To_InputFile(),
		FileName: "ping.wav",
		MimeType: "audio/wav",
	})
	if err != nil {
		t.Fatal(err)
	}
	uploaded := documentOf(t, doc)
	if uploaded.GetMimeType() != "audio/wav" {
		t.Fatalf("wav mime %s", uploaded.GetMimeType())
	}
	saved, err := a.Invoke(ctx, &mtproto.TLAccountSaveRingtone{
		Constructor: mtproto.CRC32_account_saveRingtone,
		Id: mtproto.MakeTLInputDocument(&mtproto.InputDocument{
			Id: uploaded.GetId(), AccessHash: uploaded.GetAccessHash(),
		}).To_InputDocument(),
		Unsave: mtproto.BoolFalse,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := saved.(*mtproto.TLAccountSavedRingtoneConverted); !ok {
		t.Fatalf("wav save %T", saved)
	}

	opts, err := a.Invoke(ctx, &mtproto.TLAccountInitPasskeyRegistration{
		Constructor: mtproto.CRC32_account_initPasskeyRegistration,
	})
	if err != nil {
		t.Fatal(err)
	}
	reg, ok := opts.(*mtproto.TLAccountPasskeyRegistrationOptions)
	if !ok || !strings.Contains(reg.GetOptions().GetData(), "www.tellomi.cn") {
		t.Fatalf("passkey %T", opts)
	}
}

func emojiListOf(t *testing.T, obj mtproto.TLObject) *mtproto.EmojiList {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLEmojiList:
		return v.To_EmojiList()
	case *mtproto.EmojiList:
		return v
	default:
		t.Fatalf("emoji list %T", obj)
		return nil
	}
}

func documentOf(t *testing.T, obj mtproto.TLObject) *mtproto.Document {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLDocument:
		return v.To_Document()
	case *mtproto.Document:
		return v
	default:
		t.Fatalf("document %T", obj)
		return nil
	}
}
