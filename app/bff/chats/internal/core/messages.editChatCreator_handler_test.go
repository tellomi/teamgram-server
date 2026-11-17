package core

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/chats/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/chats/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chatclient "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	srp256 "github.com/teamgram/teamgram-server/pkg/srp"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestEditChatCreatorRejectsMissingAndFakeProof(t *testing.T) {
	box := &creatorBox{chat: basicChat(90, 1, 2)}
	c := creatorCore(box)
	useMemoryPassword(t, time.Unix(1_700_000_000, 0))
	password.SeedPhone(1)

	for _, proof := range []*mtproto.InputCheckPasswordSRP{nil, emptySRP(), fakeSRP()} {
		box.called = false
		_, err := c.MessagesEditChatCreator(editCreatorReq(proof))
		if status.Convert(err).Message() != "PASSWORD_MISSING" {
			t.Fatalf("proof %v: %v", proof, err)
		}
		if box.called || box.chat.Chat.Creator != 1 {
			t.Fatalf("transfer ran: called=%v creator=%d", box.called, box.chat.Chat.Creator)
		}
	}
}

func TestEditChatCreatorChecksRealSRP(t *testing.T) {
	start := time.Unix(1_700_000_000, 0)
	now := start
	prev := password.Swap(password.New(password.Options{Now: func() time.Time { return now }}))
	t.Cleanup(func() { password.Swap(prev) })
	box := &creatorBox{chat: basicChat(90, 1, 2)}
	c := creatorCore(box)
	bad := setOldPassword(t, 1, "secret", &now, start)
	bad.M1[0] ^= 0xff
	if _, err := c.MessagesEditChatCreator(editCreatorReq(bad)); status.Convert(err).Message() != "PASSWORD_HASH_INVALID" {
		t.Fatalf("bad proof: %v", err)
	}
	if box.called || box.chat.Chat.Creator != 1 {
		t.Fatalf("bad proof transferred creator=%d called=%v", box.chat.Chat.Creator, box.called)
	}

	good := freshProof(t, 1, "secret")
	if _, err := c.MessagesEditChatCreator(editCreatorReq(good)); err != nil {
		t.Fatal(err)
	}
	if !box.called || box.chat.Chat.Creator != 2 {
		t.Fatalf("valid proof creator=%d called=%v", box.chat.Chat.Creator, box.called)
	}
}

func creatorCore(box *creatorBox) *ChatsCore {
	return &ChatsCore{
		ctx: context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			ChatClient: chatclient.NewDirectChatClient(box),
		}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1, PermAuthKeyId: 7},
	}
}

func editCreatorReq(proof *mtproto.InputCheckPasswordSRP) *mtproto.TLMessagesEditChatCreator {
	return &mtproto.TLMessagesEditChatCreator{
		Constructor: mtproto.CRC32_messages_editChatCreator,
		Peer:        mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 90}).To_InputPeer(),
		UserId:      mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 1}).To_InputUser(),
		Password:    proof,
	}
}

func emptySRP() *mtproto.InputCheckPasswordSRP {
	return mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP()
}

func fakeSRP() *mtproto.InputCheckPasswordSRP {
	return mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{
		SrpId: 1,
		A:     bytes.Repeat([]byte{0x11}, 256),
		M1:    bytes.Repeat([]byte{0x22}, 32),
	}).To_InputCheckPasswordSRP()
}

func useMemoryPassword(t *testing.T, now time.Time) {
	t.Helper()
	prev := password.Swap(password.New(password.Options{Now: func() time.Time { return now }}))
	t.Cleanup(func() { password.Swap(prev) })
}

func setOldPassword(t *testing.T, userID int64, plain string, now *time.Time, start time.Time) *mtproto.InputCheckPasswordSRP {
	t.Helper()
	*now = start
	password.SeedPhone(userID)
	pw, err := password.AccountPassword(context.Background(), userID, 0)
	if err != nil {
		t.Fatal(err)
	}
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x11}, 32)...)
	_, err = password.Update(context.Background(), userID, 0, emptySRP(), &mtproto.Account_PasswordInputSettings{
		NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
			Salt1: salt1,
			Salt2: append([]byte{}, pw.GetNewAlgo().GetSalt2()...),
			G:     pw.GetNewAlgo().GetG(),
			P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
		}).To_PasswordKdfAlgo(),
		NewPasswordHash: srp256.Verifier([]byte(plain), salt1, pw.GetNewAlgo().GetSalt2()),
		Hint:            wrapperspb.String("h"),
	})
	if err != nil {
		t.Fatal(err)
	}
	*now = start.Add(48 * time.Hour)
	current, err := password.AccountPassword(context.Background(), userID, 0)
	if err != nil {
		t.Fatal(err)
	}
	a := bytes.Repeat([]byte{0x3c}, 256)
	A, M1, err := srp256.ClientProof([]byte(plain), current.GetCurrentAlgo().GetSalt1(), current.GetCurrentAlgo().GetSalt2(), a, current.GetSrp_B())
	if err != nil {
		t.Fatal(err)
	}
	return mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{
		SrpId: current.GetSrpId().GetValue(),
		A:     A,
		M1:    M1,
	}).To_InputCheckPasswordSRP()
}

func freshProof(t *testing.T, userID int64, plain string) *mtproto.InputCheckPasswordSRP {
	t.Helper()
	current, err := password.AccountPassword(context.Background(), userID, 0)
	if err != nil {
		t.Fatal(err)
	}
	a := bytes.Repeat([]byte{0x3c}, 256)
	A, M1, err := srp256.ClientProof([]byte(plain), current.GetCurrentAlgo().GetSalt1(), current.GetCurrentAlgo().GetSalt2(), a, current.GetSrp_B())
	if err != nil {
		t.Fatal(err)
	}
	return mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{
		SrpId: current.GetSrpId().GetValue(),
		A:     A,
		M1:    M1,
	}).To_InputCheckPasswordSRP()
}

type creatorBox struct {
	chatclient.ChatClient
	chat   *mtproto.MutableChat
	called bool
}

func (c *creatorBox) ChatEditChatCreator(_ context.Context, in *chatpb.TLChatEditChatAdmin) (*mtproto.MutableChat, error) {
	c.called = true
	c.chat.Chat.Creator = in.GetEditChatAdminId()
	for _, p := range c.chat.ChatParticipants {
		switch p.UserId {
		case in.GetOperatorId():
			p.ParticipantType = mtproto.ChatMemberAdmin
		case in.GetEditChatAdminId():
			p.ParticipantType = mtproto.ChatMemberCreator
		}
	}
	return c.chat, nil
}
