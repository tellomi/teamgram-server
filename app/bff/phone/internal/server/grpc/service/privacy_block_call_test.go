package service

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/engine"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/svc"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/grpc/status"
)

func TestPhoneRequestCallPrivacyAndBlock(t *testing.T) {
	users := &callPrivacyUser{}
	s := &Service{svc: &svc.ServiceContext{
		Config: config.Config{
			PublicIP: "203.0.113.10", ReflectorPort: 3479, SFUPort: 3480,
			TurnHost: "203.0.113.10", TurnPort: 3478, StunPort: 3478,
			TurnSecret: "tellomi-turn-secret", TurnTTL: 3600,
			RingTimeout: 90, ConfirmTimeout: 30, EnableP2P: true,
		}.Normalized(),
		Users: users,
	}}
	s.eng = engine.New(engine.Options{
		Config: s.svc.Config,
		Allow:  s.allow,
		Push:   func(engine.Note) {},
		Now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
		After:  func(time.Duration, func()) func() { return func() {} },
	})

	req := &mtproto.TLPhoneRequestCall{
		Video: false, RandomId: 7,
		UserId:   mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 1}).To_InputUser(),
		GAHash:   secret(),
		Protocol: callProto(),
	}

	users.allowCall = false
	if _, err := s.PhoneRequestCall(withUser(1, 11), req); status.Convert(err).Message() != "USER_PRIVACY_RESTRICTED" {
		t.Fatalf("privacy deny: %v", err)
	}

	users.allowCall = true
	users.blocked = true
	req.RandomId = 8
	if _, err := s.PhoneRequestCall(withUser(1, 11), req); status.Convert(err).Message() != "USER_PRIVACY_RESTRICTED" {
		t.Fatalf("block deny: %v", err)
	}

	users.blocked = false
	req.RandomId = 9
	got, err := s.PhoneRequestCall(withUser(1, 11), req)
	if err != nil || got.GetPhoneCall().GetPredicateName() != mtproto.Predicate_phoneCallWaiting {
		t.Fatalf("allowed call: %v %s", err, got.GetPhoneCall().GetPredicateName())
	}
}

type callPrivacyUser struct {
	userclient.UserClient
	allowCall bool
	blocked   bool
}

func (u *callPrivacyUser) UserCheckPrivacy(_ context.Context, in *user.TLUserCheckPrivacy) (*mtproto.Bool, error) {
	if in.GetKeyType() == int32(mtproto.PHONE_CALL) && !u.allowCall {
		return mtproto.BoolFalse, nil
	}
	return mtproto.BoolTrue, nil
}

func (u *callPrivacyUser) UserBlockedEitherWay(_ context.Context, in *user.TLUserCheckBlockUserList) (*user.Vector_Long, error) {
	if u.blocked {
		return &user.Vector_Long{Datas: append([]int64{}, in.GetId()...)}, nil
	}
	return &user.Vector_Long{}, nil
}
