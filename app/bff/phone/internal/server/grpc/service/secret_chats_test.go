package service

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/dh"
	"github.com/teamgram/teamgram-server/app/bff/phone/secretchat"
	"google.golang.org/grpc/status"
)

func TestSecretChatFlow(t *testing.T) {
	secretchat.Reset()
	t.Cleanup(secretchat.Reset)
	svc := NewWithEngine(nil)
	var pushed []struct {
		user int64
		upd  *mtproto.Updates
	}
	svc.secretPush = func(userID int64, upd *mtproto.Updates) {
		pushed = append(pushed, struct {
			user int64
			upd  *mtproto.Updates
		}{userID, upd})
	}
	admin := withUser(1, 11)
	peerUser := withUser(2, 22)
	ga := dh.Public(secret())
	gb := dh.Public(secret())

	if _, err := svc.MessagesRequestEncryption(admin, &mtproto.TLMessagesRequestEncryption{
		UserId: mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser(), GA: ga, RandomId: 7,
	}); status.Convert(err).Message() != "USER_ID_INVALID" {
		t.Fatalf("self: %v", err)
	}
	if _, err := svc.MessagesRequestEncryption(admin, &mtproto.TLMessagesRequestEncryption{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser(), RandomId: 7, GA: []byte{1, 2, 3},
	}); status.Convert(err).Message() != "DH_G_A_INVALID" {
		t.Fatalf("ga: %v", err)
	}

	waiting, err := svc.MessagesRequestEncryption(admin, &mtproto.TLMessagesRequestEncryption{
		UserId:   mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser(),
		RandomId: 42, GA: ga,
	})
	if err != nil {
		t.Fatal(err)
	}
	if waiting.GetPredicateName() != mtproto.Predicate_encryptedChatWaiting || waiting.GetId() != 42 || waiting.GetAdminId() != 1 || waiting.GetParticipantId() != 2 {
		t.Fatalf("waiting %+v", waiting)
	}
	roundTrip(t, waiting, 0x66b25953)
	if len(pushed) != 1 || pushed[0].user != 2 || pushed[0].upd.Updates[0].GetChat().GetPredicateName() != mtproto.Predicate_encryptedChatRequested {
		t.Fatalf("request push %+v", pushed)
	}
	roundTrip(t, pushed[0].upd.Updates[0].GetChat(), 0x48f1d94c)
	if _, err := svc.MessagesRequestEncryption(admin, &mtproto.TLMessagesRequestEncryption{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser(), RandomId: 42, GA: ga,
	}); status.Convert(err).Message() != "RANDOM_ID_DUPLICATE" {
		t.Fatalf("dup: %v", err)
	}

	encPeer := mtproto.MakeTLInputEncryptedChat(&mtproto.InputEncryptedChat{
		ChatId: waiting.GetId(), AccessHash: waiting.GetAccessHash(),
	}).To_InputEncryptedChat()
	if _, err := svc.MessagesAcceptEncryption(admin, &mtproto.TLMessagesAcceptEncryption{
		Peer: encPeer, GB: gb, KeyFingerprint: 99,
	}); status.Convert(err).Message() != "CHAT_ID_INVALID" {
		t.Fatalf("admin accept: %v", err)
	}
	accepted, err := svc.MessagesAcceptEncryption(peerUser, &mtproto.TLMessagesAcceptEncryption{
		Peer: encPeer, GB: gb, KeyFingerprint: 99,
	})
	if err != nil {
		t.Fatal(err)
	}
	if accepted.GetPredicateName() != mtproto.Predicate_encryptedChat || accepted.GetKeyFingerprint() != 99 {
		t.Fatalf("accepted %+v", accepted)
	}
	roundTrip(t, accepted, 0x61f0d4c7)
	if string(accepted.GetGAOrB()) != string(ga) {
		t.Fatal("acceptor should receive g_a")
	}
	adminUp := pushed[len(pushed)-1]
	if adminUp.user != 1 || string(adminUp.upd.Updates[0].GetChat().GetGAOrB()) != string(gb) {
		t.Fatal("admin should receive g_b")
	}
	if _, err := svc.MessagesAcceptEncryption(peerUser, &mtproto.TLMessagesAcceptEncryption{
		Peer: encPeer, GB: gb, KeyFingerprint: 99,
	}); status.Convert(err).Message() != "ENCRYPTION_ALREADY_ACCEPTED" {
		t.Fatalf("second accept: %v", err)
	}

	if _, err := svc.MessagesSetEncryptedTyping(admin, &mtproto.TLMessagesSetEncryptedTyping{
		Peer: encPeer, Typing: mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	typing := pushed[len(pushed)-1]
	if typing.user != 2 || typing.upd.Updates[0].GetPredicateName() != mtproto.Predicate_updateEncryptedChatTyping {
		t.Fatalf("typing %+v", typing.upd.Updates[0])
	}
	roundTrip(t, typing.upd.Updates[0], 0x1710f156)

	if _, err := svc.MessagesReadEncryptedHistory(peerUser, &mtproto.TLMessagesReadEncryptedHistory{
		Peer: encPeer, MaxDate: 50,
	}); err != nil {
		t.Fatal(err)
	}
	read := pushed[len(pushed)-1]
	if read.user != 1 || read.upd.Updates[0].GetMaxDate() != 50 {
		t.Fatalf("read %+v", read.upd.Updates[0])
	}
	roundTrip(t, read.upd.Updates[0], 0x38fe25b7)

	sent, err := svc.MessagesSendEncrypted(admin, &mtproto.TLMessagesSendEncrypted{
		Peer: encPeer, RandomId: 1001, Data: []byte("cipher-text-payload"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if sent.GetPredicateName() != mtproto.Predicate_messages_sentEncryptedMessage || sent.GetDate() == 0 {
		t.Fatalf("sent %+v", sent)
	}
	roundTrip(t, sent, 0x560f8935)
	msgUp := pushed[len(pushed)-1]
	if msgUp.user != 2 || msgUp.upd.Updates[0].GetQts() != 0 {
		t.Fatalf("qts %+v", msgUp.upd.Updates[0])
	}
	if msgUp.upd.Updates[0].GetMessage_ENCRYPTEDMESSAGE().GetFile().GetPredicateName() != mtproto.Predicate_encryptedFileEmpty {
		t.Fatal("text send should carry encryptedFileEmpty")
	}
	roundTrip(t, msgUp.upd.Updates[0], 0x12bcbd9a)
	again, err := svc.MessagesSendEncrypted(admin, &mtproto.TLMessagesSendEncrypted{
		Peer: encPeer, RandomId: 1001, Data: []byte("cipher-text-payload"),
	})
	if err != nil || again.GetDate() != sent.GetDate() {
		t.Fatalf("dedup %+v %v", again, err)
	}

	file, err := svc.MessagesSendEncryptedFile(peerUser, &mtproto.TLMessagesSendEncryptedFile{
		Peer: encPeer, RandomId: 1002, Data: []byte("cipher-file"),
		File: mtproto.MakeTLInputEncryptedFile(&mtproto.InputEncryptedFile{Id: 9, AccessHash: 8, KeyFingerprint: 7}).To_InputEncryptedFile(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if file.GetPredicateName() != mtproto.Predicate_messages_sentEncryptedFile || file.GetFile().GetId() != 9 || file.GetFile().GetDcId() != 1 {
		t.Fatalf("file %+v", file)
	}
	roundTrip(t, file, 0x9493ff32)
	if _, err := svc.MessagesSendEncryptedFile(peerUser, &mtproto.TLMessagesSendEncryptedFile{
		Peer: encPeer, RandomId: 1003, Data: []byte("cipher-file"),
		File: mtproto.MakeTLInputEncryptedFileEmpty(&mtproto.InputEncryptedFile{}).To_InputEncryptedFile(),
	}); status.Convert(err).Message() != "FILE_EMTPY" {
		t.Fatalf("empty file: %v", err)
	}
	if _, err := svc.MessagesSendEncryptedFile(peerUser, &mtproto.TLMessagesSendEncryptedFile{
		Peer: encPeer, RandomId: 1004, Data: []byte("cipher-file"),
		File: mtproto.MakeTLInputEncryptedFileUploaded(&mtproto.InputEncryptedFile{
			Id: 3, Parts: 1, Md5Checksum: "not-an-md5",
		}).To_InputEncryptedFile(),
	}); status.Convert(err).Message() != "MD5_CHECKSUM_INVALID" {
		t.Fatalf("md5: %v", err)
	}

	svcMsg, err := svc.MessagesSendEncryptedService(admin, &mtproto.TLMessagesSendEncryptedService{
		Peer: encPeer, RandomId: 1005, Data: []byte("cipher-service"),
	})
	if err != nil || svcMsg.GetPredicateName() != mtproto.Predicate_messages_sentEncryptedMessage {
		t.Fatalf("service %+v %v", svcMsg, err)
	}
	svcUp := pushed[len(pushed)-1].upd.Updates[0].GetMessage_ENCRYPTEDMESSAGE()
	if svcUp.GetPredicateName() != mtproto.Predicate_encryptedMessageService {
		t.Fatalf("service update %s", svcUp.GetPredicateName())
	}
	roundTrip(t, svcUp, 0x23734b06)

	if _, err := svc.MessagesSendEncrypted(admin, &mtproto.TLMessagesSendEncrypted{
		Peer: encPeer, RandomId: 1006, Data: nil,
	}); status.Convert(err).Message() != "DATA_INVALID" {
		t.Fatalf("empty data: %v", err)
	}
	if _, err := svc.MessagesSendEncrypted(admin, &mtproto.TLMessagesSendEncrypted{
		Peer: encPeer, RandomId: 1007, Data: make([]byte, secretchat.MaxData+1),
	}); status.Convert(err).Message() != "DATA_TOO_LONG" {
		t.Fatalf("long: %v", err)
	}

	ok, err := svc.MessagesDiscardEncryption(admin, &mtproto.TLMessagesDiscardEncryption{ChatId: 42, DeleteHistory: true})
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err)
	}
	disc := pushed[len(pushed)-1].upd.Updates[0].GetChat()
	if disc.GetPredicateName() != mtproto.Predicate_encryptedChatDiscarded || !disc.GetHistoryDeleted() {
		t.Fatalf("discard %+v", disc)
	}
	roundTrip(t, disc, 0x1e1c7c45)
	if _, err := svc.MessagesSendEncrypted(peerUser, &mtproto.TLMessagesSendEncrypted{
		Peer: encPeer, RandomId: 1008, Data: []byte("nope"),
	}); status.Convert(err).Message() != "ENCRYPTION_DECLINED" {
		t.Fatalf("after discard: %v", err)
	}
	if _, err := svc.MessagesDiscardEncryption(admin, &mtproto.TLMessagesDiscardEncryption{ChatId: 42}); status.Convert(err).Message() != "ENCRYPTION_ALREADY_DECLINED" {
		t.Fatalf("second discard: %v", err)
	}
	if _, err := svc.MessagesDiscardEncryption(admin, &mtproto.TLMessagesDiscardEncryption{}); status.Convert(err).Message() != "CHAT_ID_EMPTY" {
		t.Fatalf("empty chat: %v", err)
	}
	if _, err := svc.MessagesDiscardEncryption(admin, &mtproto.TLMessagesDiscardEncryption{ChatId: 99}); status.Convert(err).Message() != "ENCRYPTION_ID_INVALID" {
		t.Fatalf("unknown: %v", err)
	}

	found, err := svc.MessagesDeletePhoneCallHistory(admin, &mtproto.TLMessagesDeletePhoneCallHistory{Revoke: true})
	if err != nil || found.GetPredicateName() != mtproto.Predicate_messages_affectedFoundMessages || found.GetMessages() == nil {
		t.Fatalf("call history %+v %v", found, err)
	}
	roundTrip(t, found, 0xef8d3e6c)

	if err := secretchat.Default().Report(2, encPeer); err != nil {
		t.Fatal(err)
	}
	bad := mtproto.MakeTLInputEncryptedChat(&mtproto.InputEncryptedChat{ChatId: 42, AccessHash: 1}).To_InputEncryptedChat()
	if err := secretchat.Default().Report(2, bad); status.Convert(err).Message() != "CHAT_ID_INVALID" {
		t.Fatalf("report: %v", err)
	}
	_ = time.Second
}
