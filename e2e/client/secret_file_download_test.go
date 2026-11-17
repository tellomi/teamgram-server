package e2e

import (
	"bytes"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"math/big"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSecretFileDownload is ISSUE-S-SECRET-ENCFILE-001. After a secret chat
// accepts a 4096-byte upload, both participants download those bytes with
// upload.getFile. A wrong access_hash and an unknown id are FILE_ID_INVALID.
// inputEncryptedFile reuses the stored object. Phones 9996610691 and
// 9996610692 are unused elsewhere in this repo.
func TestSecretFileDownload(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()

	a, _ := dialPhone(t, ctx, cfg, "9996610691", "EncA")
	b, bSelf := dialPhone(t, ctx, cfg, "9996610692", "EncB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610692", "EncB")
	if err != nil {
		t.Fatal(err)
	}
	if bID != bSelf.ID {
		t.Fatalf("imported peer %d, session user %d", bID, bSelf.ID)
	}
	if _, _, err = b.ImportPhone(ctx, "9996610691", "EncA"); err != nil {
		t.Fatal(err)
	}

	aPts, aQts, aDate, err := a.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	bPts, bQts, bDate, err := b.State(ctx)
	if err != nil {
		t.Fatal(err)
	}

	prime, generator := dhConfig(t, a)
	aSecret := randomBytes(t, 32)
	ga := dhPublic(prime, generator, aSecret)
	chatID := positiveInt32(t)
	waiting := invokeOK(t, a, &mtproto.TLMessagesRequestEncryption{
		Constructor: mtproto.CRC32_messages_requestEncryption,
		UserId:      inputUser(bID, bHash),
		RandomId:    chatID,
		GA:          ga,
	})
	requested := encryptedChatOf(waiting)
	if requested.GetId() == 0 || requested.GetAccessHash() == 0 {
		t.Fatalf("requestEncryption returned %T id=%d", waiting, requested.GetId())
	}
	chatID = requested.GetId()
	access := requested.GetAccessHash()

	bDiff, err := b.Difference(ctx, bPts, bQts, bDate)
	if err != nil {
		t.Fatal(err)
	}
	seen := findEncryptedChat(bDiff, mtproto.Predicate_encryptedChatRequested, chatID)
	if seen.GetId() != chatID || len(seen.GetGA()) == 0 {
		t.Fatalf("participant did not receive the request: %T", bDiff)
	}

	bSecret := randomBytes(t, 32)
	gb := dhPublic(prime, generator, bSecret)
	shared := dhShared(seen.GetGA(), bSecret, prime)
	accepted := invokeOK(t, b, &mtproto.TLMessagesAcceptEncryption{
		Constructor:    mtproto.CRC32_messages_acceptEncryption,
		Peer:           inputEncryptedChat(chatID, seen.GetAccessHash()),
		GB:             gb,
		KeyFingerprint: dhFingerprint(shared),
	})
	if encryptedChatOf(accepted).GetId() != chatID {
		t.Fatalf("acceptEncryption returned %T", accepted)
	}
	aDiff, err := a.Difference(ctx, aPts, aQts, aDate)
	if err != nil {
		t.Fatal(err)
	}
	if findEncryptedChat(aDiff, mtproto.Predicate_encryptedChat, chatID).GetId() != chatID {
		t.Fatalf("admin did not receive the accepted chat: %T", aDiff)
	}

	peer := inputEncryptedChat(chatID, access)
	t.Cleanup(func() {
		cx, cancel := contextTimeout(20 * time.Second)
		defer cancel()
		_, _ = a.Invoke(cx, &mtproto.TLMessagesDiscardEncryption{
			Constructor:   mtproto.CRC32_messages_discardEncryption,
			DeleteHistory: true,
			ChatId:        chatID,
		})
	})

	payload := bytes.Repeat([]byte{0x5a}, 4096)
	sum := md5.Sum(payload)
	checksum := hex.EncodeToString(sum[:])
	partID := randomID()
	invokeOK(t, a, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      partID,
		FilePart:    0,
		Bytes:       payload,
	})
	const fingerprint int32 = 0x13572468
	bPts, bQts, bDate, err = b.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sent := invokeOK(t, a, &mtproto.TLMessagesSendEncryptedFile{
		Constructor: mtproto.CRC32_messages_sendEncryptedFile,
		Peer:        peer,
		RandomId:    randomID(),
		Data:        []byte("tellomi-secret-file"),
		File: mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{
			Id: partID, Parts: 1, Md5Checksum: checksum, KeyFingerprint: fingerprint,
		}).To_InputEncryptedFile(),
	})
	file := sentEncryptedFile(t, sent)
	if file.GetId() == 0 || file.GetAccessHash() == 0 || file.GetSize2_INT64() != 4096 || file.GetKeyFingerprint() != fingerprint {
		t.Fatalf("sent file id=%d size=%d kf=%d", file.GetId(), file.GetSize2_INT64(), file.GetKeyFingerprint())
	}

	bDiff, err = b.Difference(ctx, bPts, bQts, bDate)
	if err != nil {
		t.Fatal(err)
	}
	got := encryptedFileFor(bDiff, file.GetId())
	if got.GetSize2_INT64() != 4096 || got.GetAccessHash() != file.GetAccessHash() {
		t.Fatalf("peer file size=%d access match=%v", got.GetSize2_INT64(), got.GetAccessHash() == file.GetAccessHash())
	}

	aBytes := downloadEncrypted(t, a, file.GetId(), file.GetAccessHash())
	bBytes := downloadEncrypted(t, b, file.GetId(), got.GetAccessHash())
	if !bytes.Equal(aBytes, payload) || !bytes.Equal(bBytes, payload) {
		t.Fatalf("download len a=%d b=%d", len(aBytes), len(bBytes))
	}

	expectRPC(t, a, encryptedGet(file.GetId(), file.GetAccessHash()^1), "FILE_ID_INVALID")
	expectRPC(t, b, encryptedGet(file.GetId()^0x5a5a, file.GetAccessHash()), "FILE_ID_INVALID")

	reused := invokeOK(t, a, &mtproto.TLMessagesSendEncryptedFile{
		Constructor: mtproto.CRC32_messages_sendEncryptedFile,
		Peer:        peer,
		RandomId:    randomID(),
		Data:        []byte("tellomi-secret-forward"),
		File: mtproto.MakeTLInputEncryptedFile(&mtproto.InputEncryptedFile{
			Id: file.GetId(), AccessHash: file.GetAccessHash(),
		}).To_InputEncryptedFile(),
	})
	again := sentEncryptedFile(t, reused)
	if again.GetId() != file.GetId() || again.GetSize2_INT64() != 4096 {
		t.Fatalf("reused id=%d size=%d", again.GetId(), again.GetSize2_INT64())
	}
	if !bytes.Equal(downloadEncrypted(t, b, again.GetId(), again.GetAccessHash()), payload) {
		t.Fatal("reused file bytes differ")
	}

	discarded := invokeOK(t, a, &mtproto.TLMessagesDiscardEncryption{
		Constructor:   mtproto.CRC32_messages_discardEncryption,
		DeleteHistory: true,
		ChatId:        chatID,
	})
	if !mtproto.FromBool(asBool(discarded)) {
		t.Fatalf("discardEncryption returned %T", discarded)
	}
}

func dhConfig(t *testing.T, c *Client) ([]byte, int32) {
	t.Helper()
	obj := invokeOK(t, c, &mtproto.TLMessagesGetDhConfig{
		Constructor:  mtproto.CRC32_messages_getDhConfig,
		Version:      0,
		RandomLength: 0,
	})
	cfg, ok := obj.(*mtproto.TLMessagesDhConfig)
	if !ok || len(cfg.GetP()) == 0 || cfg.GetG() == 0 {
		t.Fatalf("messages.getDhConfig returned %T", obj)
	}
	return cfg.GetP(), cfg.GetG()
}

func dhPublic(p []byte, g int32, secret []byte) []byte {
	prime := new(big.Int).SetBytes(p)
	value := new(big.Int).Exp(big.NewInt(int64(g)), new(big.Int).SetBytes(secret), prime)
	out := make([]byte, len(p))
	raw := value.Bytes()
	copy(out[len(out)-len(raw):], raw)
	return out
}

func dhShared(peerPublic, secret, p []byte) []byte {
	prime := new(big.Int).SetBytes(p)
	value := new(big.Int).Exp(new(big.Int).SetBytes(peerPublic), new(big.Int).SetBytes(secret), prime)
	out := make([]byte, len(p))
	raw := value.Bytes()
	copy(out[len(out)-len(raw):], raw)
	return out
}

func dhFingerprint(key []byte) int64 {
	sum := sha1.Sum(key)
	return int64(binary.BigEndian.Uint64(sum[len(sum)-8:]))
}

func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	out := make([]byte, n)
	if _, err := rand.Read(out); err != nil {
		t.Fatal(err)
	}
	return out
}

func positiveInt32(t *testing.T) int32 {
	t.Helper()
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	n := int32(binary.BigEndian.Uint32(b[:]) & 0x7fffffff)
	if n == 0 {
		return 1
	}
	return n
}

func inputEncryptedChat(id int32, access int64) *mtproto.InputEncryptedChat {
	return mtproto.MakeTLInputEncryptedChat(&mtproto.InputEncryptedChat{
		ChatId: id, AccessHash: access,
	}).To_InputEncryptedChat()
}

func encryptedGet(id, access int64) *mtproto.TLUploadGetFile {
	return &mtproto.TLUploadGetFile{
		Constructor: mtproto.CRC32_upload_getFile_be5335be,
		Location: mtproto.MakeTLInputEncryptedFileLocation(&mtproto.InputFileLocation{
			Id: id, AccessHash: access,
		}).To_InputFileLocation(),
		Offset_INT64: 0,
		Limit:        4096,
	}
}

func downloadEncrypted(t *testing.T, c *Client, id, access int64) []byte {
	t.Helper()
	obj := invokeOK(t, c, encryptedGet(id, access))
	body := bytesOf(obj)
	if len(body) != 4096 {
		t.Fatalf("upload.getFile returned %d bytes (%T)", len(body), obj)
	}
	return body
}

func sentEncryptedFile(t *testing.T, obj mtproto.TLObject) *mtproto.EncryptedFile {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesSentEncryptedFile:
		if v.GetFile() != nil {
			return v.GetFile()
		}
	case *mtproto.Messages_SentEncryptedMessage:
		if v.GetFile() != nil {
			return v.GetFile()
		}
	}
	t.Fatalf("sendEncryptedFile returned %T", obj)
	return nil
}

func encryptedChatOf(obj mtproto.TLObject) *mtproto.EncryptedChat {
	switch v := obj.(type) {
	case *mtproto.TLEncryptedChatWaiting:
		return v.To_EncryptedChat()
	case *mtproto.TLEncryptedChatRequested:
		return v.To_EncryptedChat()
	case *mtproto.TLEncryptedChat:
		return v.To_EncryptedChat()
	case *mtproto.EncryptedChat:
		return v
	default:
		return nil
	}
}

func findEncryptedChat(obj mtproto.TLObject, predicate string, id int32) *mtproto.EncryptedChat {
	_, ups := secretPage(obj)
	for _, u := range ups {
		if u.GetPredicateName() != mtproto.Predicate_updateEncryption {
			continue
		}
		ch := u.GetChat()
		if ch.GetId() == id && ch.GetPredicateName() == predicate {
			return ch
		}
	}
	return nil
}

func encryptedFileFor(obj mtproto.TLObject, id int64) *mtproto.EncryptedFile {
	msgs, _ := secretPage(obj)
	for _, m := range msgs {
		if m.GetFile().GetId() == id {
			return m.GetFile()
		}
	}
	return nil
}

func secretPage(obj mtproto.TLObject) ([]*mtproto.EncryptedMessage, []*mtproto.Update) {
	switch v := obj.(type) {
	case *mtproto.TLUpdatesDifference:
		return v.GetNewEncryptedMessages(), v.GetOtherUpdates()
	case *mtproto.TLUpdatesDifferenceSlice:
		return v.GetNewEncryptedMessages(), v.GetOtherUpdates()
	case *mtproto.Updates_Difference:
		return v.GetNewEncryptedMessages(), v.GetOtherUpdates()
	default:
		return nil, nil
	}
}

func asBool(obj mtproto.TLObject) *mtproto.Bool {
	switch v := obj.(type) {
	case *mtproto.TLBoolTrue:
		return v.To_Bool()
	case *mtproto.TLBoolFalse:
		return v.To_Bool()
	case *mtproto.Bool:
		return v
	default:
		return nil
	}
}
