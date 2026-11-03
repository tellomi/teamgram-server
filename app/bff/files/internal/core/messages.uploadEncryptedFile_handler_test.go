package core

import (
	"context"
	"encoding/binary"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/phone/secretchat"

	"google.golang.org/grpc/status"
)

func TestUploadEncryptedFile(t *testing.T) {
	secretchat.Reset()
	t.Cleanup(secretchat.Reset)
	peer := acceptedSecret(t, 1, 2, 42)
	var gotCreator int64
	var gotParts int32
	uploadEncryptedPartsHook = func(_ context.Context, creator int64, file *mtproto.InputEncryptedFile) (*mtproto.EncryptedFile, error) {
		gotCreator = creator
		gotParts = file.GetParts()
		return mtproto.MakeTLEncryptedFile(&mtproto.EncryptedFile{
			Id: 100, AccessHash: 200, Size2_INT64: 4, Size2_INT32: 4, DcId: 1,
		}).To_EncryptedFile(), nil
	}
	t.Cleanup(func() { uploadEncryptedPartsHook = nil })

	c := fileCore()
	c.MD = &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 77}
	md5 := "0123456789abcdef0123456789abcdef"
	out, err := c.MessagesUploadEncryptedFile(&mtproto.TLMessagesUploadEncryptedFile{
		Peer: peer,
		File: mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{
			Id: 5, Parts: 1, Md5Checksum: md5, KeyFingerprint: 33,
		}).To_InputEncryptedFile(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotCreator != 77 || gotParts != 1 {
		t.Fatalf("dfs call creator=%d parts=%d", gotCreator, gotParts)
	}
	if out.GetPredicateName() != mtproto.Predicate_encryptedFile || out.GetId() != 100 || out.GetAccessHash() != 200 {
		t.Fatalf("file %+v", out)
	}
	if out.GetSize2_INT64() != 4 || out.GetDcId() != 1 || out.GetKeyFingerprint() != 33 {
		t.Fatalf("meta size=%d dc=%d fp=%d", out.GetSize2_INT64(), out.GetDcId(), out.GetKeyFingerprint())
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := mtproto.MakeTLEncryptedFile(out).Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	if binary.LittleEndian.Uint32(raw[:4]) != 0xa8008cd8 {
		t.Fatalf("constructor 0x%x", binary.LittleEndian.Uint32(raw[:4]))
	}
	kept, chatID, ok := secretchat.Default().LookupEncrypted(100)
	if !ok || chatID != 42 || kept.GetKeyFingerprint() != 33 || kept.GetSize2_INT64() != 4 {
		t.Fatalf("kept %+v chat %d ok %v", kept, chatID, ok)
	}

	again, err := c.MessagesUploadEncryptedFile(&mtproto.TLMessagesUploadEncryptedFile{
		Peer: peer,
		File: mtproto.MakeTLInputEncryptedFile(&mtproto.InputEncryptedFile{Id: 100, AccessHash: 200}).To_InputEncryptedFile(),
	})
	if err != nil || again.GetId() != 100 || again.GetKeyFingerprint() != 33 {
		t.Fatalf("again %+v %v", again, err)
	}
	if _, err := c.MessagesUploadEncryptedFile(&mtproto.TLMessagesUploadEncryptedFile{
		Peer: peer,
		File: mtproto.MakeTLInputEncryptedFile(&mtproto.InputEncryptedFile{Id: 100, AccessHash: 1}).To_InputEncryptedFile(),
	}); status.Convert(err).Message() != "FILE_ID_INVALID" {
		t.Fatalf("hash: %v", err)
	}

	big, err := c.MessagesUploadEncryptedFile(&mtproto.TLMessagesUploadEncryptedFile{
		Peer: peer,
		File: mtproto.MakeTLInputEncryptedFileBigUploaded(&mtproto.InputEncryptedFile{
			Id: 9, Parts: 2, KeyFingerprint: 44,
		}).To_InputEncryptedFile(),
	})
	if err != nil || big.GetKeyFingerprint() != 44 || gotParts != 2 {
		t.Fatalf("big %+v parts %d %v", big, gotParts, err)
	}
}

func TestUploadEncryptedFileRejects(t *testing.T) {
	secretchat.Reset()
	t.Cleanup(secretchat.Reset)
	uploadEncryptedPartsHook = func(context.Context, int64, *mtproto.InputEncryptedFile) (*mtproto.EncryptedFile, error) {
		t.Fatal("dfs should not be called")
		return nil, nil
	}
	t.Cleanup(func() { uploadEncryptedPartsHook = nil })
	peer := acceptedSecret(t, 1, 2, 42)
	c := fileCore()
	c.MD = &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 77}

	cases := []struct {
		name string
		md   *metadata.RpcMetadata
		peer *mtproto.InputEncryptedChat
		file *mtproto.InputEncryptedFile
		want string
	}{
		{"nil session", nil, peer, uploaded(1, 1), "AUTH_KEY_UNREGISTERED"},
		{"bot", &metadata.RpcMetadata{UserId: 1, IsBot: true}, peer, uploaded(1, 1), "BOT_METHOD_INVALID"},
		{"stranger", &metadata.RpcMetadata{UserId: 9}, peer, uploaded(1, 1), "CHAT_ID_INVALID"},
		{"empty", c.MD, peer, mtproto.MakeTLInputEncryptedFileEmpty(&mtproto.InputEncryptedFile{}).To_InputEncryptedFile(), "FILE_EMTPY"},
		{"zero id", c.MD, peer, uploaded(0, 1), "FILE_EMTPY"},
		{"parts", c.MD, peer, uploaded(3, 0), "FILE_PARTS_INVALID"},
		{"md5", c.MD, peer, mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{
			Id: 3, Parts: 1, Md5Checksum: "not-an-md5",
		}).To_InputEncryptedFile(), "MD5_CHECKSUM_INVALID"},
	}
	for _, tc := range cases {
		core := fileCore()
		core.MD = tc.md
		_, err := core.MessagesUploadEncryptedFile(&mtproto.TLMessagesUploadEncryptedFile{Peer: tc.peer, File: tc.file})
		if status.Convert(err).Message() != tc.want {
			t.Errorf("%s: %v", tc.name, err)
		}
	}

	waiting, _, err := secretchat.Default().Request(1, 3, 70, []byte{1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	waitPeer := mtproto.MakeTLInputEncryptedChat(&mtproto.InputEncryptedChat{
		ChatId: waiting.GetId(), AccessHash: waiting.GetAccessHash(),
	}).To_InputEncryptedChat()
	if _, err := c.MessagesUploadEncryptedFile(&mtproto.TLMessagesUploadEncryptedFile{Peer: waitPeer, File: uploaded(3, 1)}); status.Convert(err).Message() != "CHAT_ID_INVALID" {
		t.Fatalf("waiting: %v", err)
	}
	if _, err := secretchat.Default().Discard(1, 42, false, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := c.MessagesUploadEncryptedFile(&mtproto.TLMessagesUploadEncryptedFile{Peer: peer, File: uploaded(3, 1)}); status.Convert(err).Message() != "ENCRYPTION_DECLINED" {
		t.Fatalf("discarded: %v", err)
	}
	uploadEncryptedPartsHook = func(context.Context, int64, *mtproto.InputEncryptedFile) (*mtproto.EncryptedFile, error) {
		return nil, mtproto.NewFilePartXMissing(0)
	}
	live := acceptedSecret(t, 1, 2, 80)
	_, err = c.MessagesUploadEncryptedFile(&mtproto.TLMessagesUploadEncryptedFile{Peer: live, File: uploaded(3, 1)})
	if status.Convert(err).Message() != "FILE_PART_0_MISSING" {
		t.Fatal(err)
	}
}

func acceptedSecret(t *testing.T, admin, peer int64, id int32) *mtproto.InputEncryptedChat {
	t.Helper()
	chat, _, err := secretchat.Default().Request(admin, peer, id, []byte{1}, 1)
	if err != nil {
		t.Fatal(err)
	}
	in := mtproto.MakeTLInputEncryptedChat(&mtproto.InputEncryptedChat{
		ChatId: chat.GetId(), AccessHash: chat.GetAccessHash(),
	}).To_InputEncryptedChat()
	if _, _, err = secretchat.Default().Accept(peer, in, []byte{2}, 3, 2); err != nil {
		t.Fatal(err)
	}
	return in
}

func uploaded(id int64, parts int32) *mtproto.InputEncryptedFile {
	return mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{
		Id: id, Parts: parts, Md5Checksum: "0123456789abcdef0123456789abcdef", KeyFingerprint: 7,
	}).To_InputEncryptedFile()
}
