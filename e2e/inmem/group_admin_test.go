package inmem

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/chatinvites/joinhide"
	"github.com/teamgram/teamgram-server/app/bff/chats/owner"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/contactphoto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	srp256 "github.com/teamgram/teamgram-server/pkg/srp"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func useChannel(t *testing.T) (*channel.Service, context.Context) {
	t.Helper()
	prev := channel.Shared()
	svc := channel.New(channel.NewMemoryStore())
	channel.Use(svc)
	t.Cleanup(func() { channel.Use(prev) })
	return svc, context.Background()
}

func megagroup(t *testing.T, svc *channel.Service, ctx context.Context) *mtproto.Chat {
	t.Helper()
	up, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Owners", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	return up.Chats[0]
}

func channelPeer(ch *mtproto.Chat) *mtproto.PeerUtil {
	return &mtproto.PeerUtil{
		PeerType:   mtproto.PEER_CHANNEL,
		PeerId:     ch.Id,
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}
}

func oldCloudProof(t *testing.T, userID int64, plain string) *mtproto.InputCheckPasswordSRP {
	t.Helper()
	start := time.Unix(1_700_000_000, 0)
	now := start
	svc := password.New(password.Options{Now: func() time.Time { return now }})
	prev := password.Swap(svc)
	t.Cleanup(func() { password.Swap(prev) })
	password.SeedPhone(userID)
	pw, err := password.AccountPassword(context.Background(), userID, 0)
	if err != nil {
		t.Fatal(err)
	}
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x11}, 32)...)
	_, err = password.Update(context.Background(), userID, 0, mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP(), &mtproto.Account_PasswordInputSettings{
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
	now = start.Add(48 * time.Hour)
	current, err := password.AccountPassword(context.Background(), userID, 0)
	if err != nil {
		t.Fatal(err)
	}
	a := bytes.Repeat([]byte{0x3c}, 256)
	A, M1, err := srp256.ClientProof([]byte(plain), current.GetCurrentAlgo().GetSalt1(), current.GetCurrentAlgo().GetSalt2(), a, current.GetSrp_B())
	if err != nil {
		t.Fatal(err)
	}
	return mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{SrpId: current.GetSrpId().GetValue(), A: A, M1: M1}).To_InputCheckPasswordSRP()
}

func srp() *mtproto.InputCheckPasswordSRP {
	return mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{A: []byte{1, 2, 3}, M1: []byte{4}}).To_InputCheckPasswordSRP()
}

func TestTransferChannelCreator(t *testing.T) {
	svc, ctx := useChannel(t)
	ch := megagroup(t, svc, ctx)
	peer := channelPeer(ch)
	if _, err := svc.Invite(ctx, 1, 0, mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: ch.Id, AccessHash: peer.AccessHash}).To_InputChannel(), []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	next, err := owner.FutureChannel(ctx, 1, peer)
	if err != nil || next != 2 {
		t.Fatalf("future creator %d %v", next, err)
	}
	user := owner.User(1, next)
	if user.GetId() != 2 {
		t.Fatalf("user %#v", user)
	}
	if err := user.Encode(mtproto.NewEncodeBuf(128), 232); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.EditChannel(ctx, 1, 0, peer, 2, nil); status.Convert(err).Message() != "PASSWORD_MISSING" {
		t.Fatalf("empty password: %v", err)
	}
	ups, err := owner.EditChannel(ctx, 1, 0, peer, 2, oldCloudProof(t, 1, "secret"))
	if err != nil {
		t.Fatal(err)
	}
	buf := mtproto.NewEncodeBuf(8192)
	if err := ups.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	if err := (&mtproto.Updates{}).Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetParticipant(ctx, 2, mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: ch.Id, AccessHash: peer.AccessHash}).To_InputChannel(), mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer())
	if err != nil || got.Participant.GetPredicateName() != mtproto.Predicate_channelParticipantCreator {
		t.Fatalf("creator %#v %v", got, err)
	}
	if _, err := owner.FutureChannel(ctx, 1, peer); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("old creator should lose the query: %v", err)
	}
}

func TestHideAllChannelJoinRequests(t *testing.T) {
	svc, ctx := useChannel(t)
	ch := megagroup(t, svc, ctx)
	peer := channelPeer(ch)
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: ch.Id, AccessHash: peer.AccessHash}).To_InputChannel()
	if _, err := svc.ToggleJoinRequest(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Join(ctx, 4, 0, in); err == nil || !strings.Contains(err.Error(), "INVITE_REQUEST_SENT") {
		t.Fatalf("join request: %v", err)
	}
	parts, _ := svc.GetParticipants(ctx, 1, in, nil, 0, 50, 0)
	if parts.Count != 1 {
		t.Fatalf("request should not add a member, count=%d", parts.Count)
	}
	ups, err := joinhide.HideChannel(ctx, 1, 0, peer, true, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := ups.Encode(mtproto.NewEncodeBuf(4096), 232); err != nil {
		t.Fatal(err)
	}
	parts, _ = svc.GetParticipants(ctx, 1, in, nil, 0, 50, 0)
	if parts.Count != 2 {
		t.Fatalf("approved member missing, count=%d", parts.Count)
	}
	if _, err := joinhide.HideChannel(ctx, 4, 0, peer, false, ""); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member hide: %v", err)
	}
}

func TestContactProfilePhoto(t *testing.T) {
	contactphoto.Reset()
	user := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 8, AccessHash: 1}).To_InputUser()
	if _, err := contactphoto.Upload(0, &mtproto.TLPhotosUploadContactProfilePhoto{UserId: user}); status.Convert(err).Message() != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
	if _, err := contactphoto.Upload(3, &mtproto.TLPhotosUploadContactProfilePhoto{UserId: mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser(), Save: true}); status.Convert(err).Message() != "USER_ID_INVALID" {
		t.Fatal(err)
	}
	cleared, err := contactphoto.Upload(3, &mtproto.TLPhotosUploadContactProfilePhoto{UserId: user, Save: true})
	if err != nil || cleared.GetPhoto() == nil {
		t.Fatalf("clear personal photo: %v", err)
	}
	if contactphoto.Saved(3, 8) != nil {
		t.Fatal("clear left a personal photo")
	}
	if _, err := contactphoto.Upload(3, &mtproto.TLPhotosUploadContactProfilePhoto{
		UserId: user,
		Save:   true,
		File:   mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 9, Parts: 0, Name: "a.jpg"}).To_InputFile(),
	}); status.Convert(err).Message() != "FILE_PARTS_INVALID" {
		t.Fatal(err)
	}
	out, err := contactphoto.Upload(3, &mtproto.TLPhotosUploadContactProfilePhoto{
		UserId:  user,
		Save:    true,
		Suggest: true,
		File:    mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: 9, Parts: 1, Name: "a.jpg", Md5Checksum: "0123456789abcdef0123456789abcdef"}).To_InputFile(),
	})
	if err != nil || out.GetPhoto().GetId() != 9 || len(out.GetUsers()) != 2 {
		t.Fatalf("photo %#v %v", out, err)
	}
	if err := out.Encode(mtproto.NewEncodeBuf(1024), 232); err != nil {
		t.Fatal(err)
	}
	if contactphoto.Saved(3, 8).GetId() != 9 || contactphoto.Suggested(3, 8).GetId() != 9 {
		t.Fatal("photo was not stored")
	}
	if contactphoto.Saved(4, 8) != nil {
		t.Fatal("another user saw the contact photo")
	}
}

func TestBasicGroupSuccessor(t *testing.T) {
	id, err := owner.Successor(1, []owner.Seat{
		{UserID: 1, Admin: true, Date: 1, Active: true},
		{UserID: 2, Date: 3, Active: true},
		{UserID: 3, Admin: true, Date: 2, Active: true},
		{UserID: 4, Admin: true, Date: 1, Active: false},
	})
	if err != nil || id != 3 {
		t.Fatalf("successor %d %v", id, err)
	}
	if !owner.PasswordMissing(nil) || !owner.PasswordMissing(mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP()) {
		t.Fatal("empty password should be missing")
	}
	if owner.PasswordMissing(srp()) {
		t.Fatal("srp proof should be accepted")
	}
}
