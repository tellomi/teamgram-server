package channel

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/twofa/password"
	"github.com/teamgram/teamgram-server/pkg/srp"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func installOldPassword(t *testing.T, userID int64, plain string) {
	t.Helper()
	start := time.Unix(1_700_000_000, 0)
	now := start
	svc := password.New(password.Options{Now: func() time.Time { return now }})
	prev := password.Swap(svc)
	prevSession := password.SessionCreated
	t.Cleanup(func() {
		password.Swap(prev)
		password.SessionCreated = prevSession
	})
	password.SeedPhone(userID)
	ctx := context.Background()
	pw, err := password.AccountPassword(ctx, userID, 0)
	if err != nil {
		t.Fatal(err)
	}
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x11}, 32)...)
	_, err = password.Update(ctx, userID, 0, mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP(), &mtproto.Account_PasswordInputSettings{
		NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
			Salt1: salt1,
			Salt2: append([]byte{}, pw.GetNewAlgo().GetSalt2()...),
			G:     pw.GetNewAlgo().GetG(),
			P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
		}).To_PasswordKdfAlgo(),
		NewPasswordHash: srp.Verifier([]byte(plain), salt1, pw.GetNewAlgo().GetSalt2()),
		Hint:            wrapperspb.String("h"),
	})
	if err != nil {
		t.Fatal(err)
	}
	now = start.Add(48 * time.Hour)
}

func cloudProof(t *testing.T, userID int64, plain string) *mtproto.InputCheckPasswordSRP {
	t.Helper()
	pw, err := password.AccountPassword(context.Background(), userID, 0)
	if err != nil {
		t.Fatal(err)
	}
	a := bytes.Repeat([]byte{0x3c}, 256)
	A, M1, err := srp.ClientProof([]byte(plain), pw.GetCurrentAlgo().GetSalt1(), pw.GetCurrentAlgo().GetSalt2(), a, pw.GetSrp_B())
	if err != nil {
		t.Fatal(err)
	}
	return mtproto.MakeTLInputCheckPasswordSRP(&mtproto.InputCheckPasswordSRP{
		SrpId: pw.GetSrpId().GetValue(),
		A:     A,
		M1:    M1,
	}).To_InputCheckPasswordSRP()
}

func mustDecode(t *testing.T, encode func(*mtproto.EncodeBuf, int32) error, decode func(*mtproto.DecodeBuf) error) {
	t.Helper()
	buf := mtproto.NewEncodeBuf(8192)
	if err := encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	if err := decode(mtproto.NewDecodeBuf(append([]byte(nil), buf.GetBuf()...))); err != nil {
		t.Fatal(err)
	}
}

func adminFilter() *mtproto.ChannelParticipantsFilter {
	return mtproto.MakeTLChannelParticipantsAdmins(&mtproto.ChannelParticipantsFilter{}).To_ChannelParticipantsFilter()
}

func TestEditCreatorPasswordAndUpdates(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Transfer")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3}, nil); err != nil {
		t.Fatal(err)
	}
	var pushed []struct {
		user int64
		up   *mtproto.Updates
	}
	svc.SetHooks(Hooks{Push: func(_ context.Context, userID int64, _ []int64, updates *mtproto.Updates) {
		pushed = append(pushed, struct {
			user int64
			up   *mtproto.Updates
		}{userID, updates})
	}})

	if _, err := svc.EditCreator(ctx, 1, 9, in, 2, nil); status.Convert(err).Message() != "PASSWORD_MISSING" {
		t.Fatalf("missing: %v", err)
	}
	installOldPassword(t, 1, "secret")
	if _, err := svc.EditCreator(ctx, 1, 9, in, 2, cloudProof(t, 1, "nope")); status.Convert(err).Message() != "PASSWORD_HASH_INVALID" {
		t.Fatalf("wrong password: %v", err)
	}
	password.SessionCreated = func(_ context.Context, authKeyID int64) (int64, bool, error) {
		if authKeyID != 9 {
			return 0, false, nil
		}
		return time.Unix(1_700_000_000, 0).Add(48*time.Hour).Unix() - 3600, true, nil
	}
	if msg := status.Convert(mustEditErr(t, svc, ctx, in)).Message(); !strings.HasPrefix(msg, "SESSION_TOO_FRESH_") {
		t.Fatalf("session: %s", msg)
	}
	password.SessionCreated = func(_ context.Context, authKeyID int64) (int64, bool, error) {
		return time.Unix(1_700_000_000, 0).Unix(), true, nil
	}

	fresh := time.Unix(1_800_000_000, 0)
	freshNow := fresh
	freshSvc := password.New(password.Options{Now: func() time.Time { return freshNow }})
	prev := password.Swap(freshSvc)
	password.SeedPhone(1)
	setPasswordAt(t, 1, "secret")
	if msg := status.Convert(mustEditErr(t, svc, ctx, in)).Message(); !strings.HasPrefix(msg, "PASSWORD_TOO_FRESH_") {
		t.Fatalf("fresh password: %s", msg)
	}
	password.Swap(prev)
	installOldPassword(t, 1, "secret")
	password.SessionCreated = func(_ context.Context, authKeyID int64) (int64, bool, error) {
		return time.Unix(1_700_000_000, 0).Unix(), true, nil
	}

	if _, err := svc.EditCreator(ctx, 3, 9, in, 2, cloudProof(t, 1, "secret")); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member transfer: %v", err)
	}
	up, err := svc.EditCreator(ctx, 1, 9, in, 2, cloudProof(t, 1, "secret"))
	if err != nil {
		t.Fatal(err)
	}
	mustDecode(t, up.Encode, (&mtproto.Updates{}).Decode)
	sawParticipant := false
	sawChannel := false
	for _, u := range up.GetUpdates() {
		switch u.GetPredicateName() {
		case mtproto.Predicate_updateChannelParticipant:
			sawParticipant = true
			mustDecode(t, u.Encode, (&mtproto.Update{}).Decode)
		case mtproto.Predicate_updateChannel:
			sawChannel = true
		}
	}
	if !sawParticipant || !sawChannel {
		t.Fatalf("return updates %#v", up.GetUpdates())
	}
	if len(up.GetChats()) == 0 || up.GetChats()[0].GetCreator() {
		t.Fatal("former owner still marked creator")
	}
	var memberSaw bool
	for _, row := range pushed {
		if row.user != 3 {
			continue
		}
		memberSaw = true
		mustDecode(t, row.up.Encode, (&mtproto.Updates{}).Decode)
	}
	if !memberSaw {
		t.Fatal("other member was not notified")
	}
}

func mustEditErr(t *testing.T, svc *Service, ctx context.Context, in *mtproto.InputChannel) error {
	t.Helper()
	_, err := svc.EditCreator(ctx, 1, 9, in, 2, cloudProof(t, 1, "secret"))
	if err == nil {
		t.Fatal("expected an error")
	}
	return err
}

func setPasswordAt(t *testing.T, userID int64, plain string) {
	t.Helper()
	ctx := context.Background()
	pw, err := password.AccountPassword(ctx, userID, 0)
	if err != nil {
		t.Fatal(err)
	}
	salt1 := append(append([]byte{}, pw.GetNewAlgo().GetSalt1()...), bytes.Repeat([]byte{0x21}, 32)...)
	_, err = password.Update(ctx, userID, 0, mtproto.MakeTLInputCheckPasswordEmpty(&mtproto.InputCheckPasswordSRP{}).To_InputCheckPasswordSRP(), &mtproto.Account_PasswordInputSettings{
		NewAlgo: mtproto.MakeTLPasswordKdfAlgoModPow(&mtproto.PasswordKdfAlgo{
			Salt1: salt1,
			Salt2: append([]byte{}, pw.GetNewAlgo().GetSalt2()...),
			G:     pw.GetNewAlgo().GetG(),
			P:     append([]byte{}, pw.GetNewAlgo().GetP()...),
		}).To_PasswordKdfAlgo(),
		NewPasswordHash: srp.Verifier([]byte(plain), salt1, pw.GetNewAlgo().GetSalt2()),
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestAdminListConstructorAndEditScope(t *testing.T) {
	svc, ctx := newTest(t)
	ch := megagroup(t, svc, ctx, "Admins")
	in := inCh(ch)
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{2, 3, 4}, nil); err != nil {
		t.Fatal(err)
	}
	ownerRights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{
		AddAdmins: true,
		BanUsers:  true,
	}).To_ChatAdminRights()
	narrow := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{InviteUsers: true}).To_ChatAdminRights()
	banOnly := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{BanUsers: true}).To_ChatAdminRights()
	post := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{PostMessages: true}).To_ChatAdminRights()
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 2, ownerRights, "ops"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, narrow, "inv"); err != nil {
		t.Fatal(err)
	}
	list, err := svc.GetParticipants(ctx, 2, in, adminFilter(), 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	mustDecode(t, list.Encode, (&mtproto.Channels_ChannelParticipants{}).Decode)
	var selfRow *mtproto.ChannelParticipant
	for _, row := range list.GetParticipants() {
		if row.GetUserId() == 2 {
			selfRow = row
		}
		if row.GetUserId() == 3 && row.GetCanEdit() {
			t.Fatal("admin can_edit set on an owner-appointed admin")
		}
	}
	if selfRow == nil || !selfRow.GetSelf() || selfRow.GetPredicateName() != mtproto.Predicate_channelParticipantAdmin {
		t.Fatalf("self admin %#v", selfRow)
	}
	ownerList, err := svc.GetParticipants(ctx, 1, in, adminFilter(), 0, 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	mustDecode(t, ownerList.Encode, (&mtproto.Channels_ChannelParticipants{}).Decode)

	if _, err := svc.EditAdmin(ctx, 2, 0, in, 3, nil, ""); status.Convert(err).Message() != "RIGHT_FORBIDDEN" {
		t.Fatalf("demote owner admin: %v", err)
	}
	if _, err := svc.EditAdmin(ctx, 4, 0, in, 3, nil, ""); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member edit: %v", err)
	}
	if _, err := svc.EditAdmin(ctx, 2, 0, in, 4, post, ""); status.Convert(err).Message() != "RIGHT_FORBIDDEN" {
		t.Fatalf("grant beyond own rights: %v", err)
	}
	if _, err := svc.EditAdmin(ctx, 2, 0, in, 4, banOnly, "mod"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAdmin(ctx, 2, 0, in, 4, nil, ""); err != nil {
		t.Fatalf("demote own admin: %v", err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, nil, ""); err != nil {
		t.Fatalf("owner demote: %v", err)
	}
	got, err := svc.GetParticipant(ctx, 1, in, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 3}).To_InputPeer())
	if err != nil || got.GetParticipant().GetPredicateName() == mtproto.Predicate_channelParticipantAdmin {
		t.Fatalf("owner demote left an admin %#v %v", got, err)
	}
}

func TestNoForwardsToggleAndFlag(t *testing.T) {
	svc, ctx := newTest(t)
	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Quiet", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := inCh(ch)
	if _, err := svc.UpdateUsername(ctx, 1, 0, in, "quietroom"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Join(ctx, 2, 0, in); err != nil {
		t.Fatal(err)
	}
	changeInfo := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{ChangeInfo: true}).To_ChatAdminRights()
	banOnly := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{BanUsers: true}).To_ChatAdminRights()
	if _, err := svc.Invite(ctx, 1, 0, in, []int64{3, 4}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 3, changeInfo, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.EditAdmin(ctx, 1, 0, in, 4, banOnly, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ToggleNoForwards(ctx, 2, 0, in, true); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("member toggle: %v", err)
	}
	if _, err := svc.ToggleNoForwards(ctx, 4, 0, in, true); status.Convert(err).Message() != "CHAT_ADMIN_REQUIRED" {
		t.Fatalf("admin without change_info: %v", err)
	}
	up, err := svc.ToggleNoForwards(ctx, 3, 0, in, true)
	if err != nil {
		t.Fatal(err)
	}
	mustDecode(t, up.Encode, (&mtproto.Updates{}).Decode)
	if len(up.GetChats()) == 0 || !up.GetChats()[0].GetNoforwards() {
		t.Fatalf("admin toggle chat %#v", up.GetChats())
	}
	view := svc.ChatFor(ctx, 2, mustChannel(t, svc, ctx, ch.Id))
	if !view.GetNoforwards() {
		t.Fatal("member chat missing noforwards")
	}
	if _, err := svc.ToggleNoForwards(ctx, 1, 0, in, false); err != nil {
		t.Fatal(err)
	}
	if svc.ChatFor(ctx, 1, mustChannel(t, svc, ctx, ch.Id)).GetNoforwards() {
		t.Fatal("creator could not clear noforwards")
	}
	if _, err := svc.ToggleNoForwards(ctx, 1, 0, in, true); err != nil {
		t.Fatal(err)
	}
	sent, err := svc.Send(ctx, 1, ch.Id, 0, []Outgoing{{RandomID: 1, Message: &mtproto.Message{Message: "secret", Date: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	_ = sent
	loaded, err := svc.GetMessages(ctx, 2, in, []int32{1})
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.GetChats()) == 0 || !loaded.GetChats()[0].GetNoforwards() {
		t.Fatal("getMessages chat missing noforwards")
	}
}
