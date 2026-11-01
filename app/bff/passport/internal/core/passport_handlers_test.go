package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/passport/book"
	"github.com/teamgram/teamgram-server/app/bff/passport/internal/svc"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func passCore(user int64, bot bool) *PassportCore {
	return &PassportCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Book: book.New(nil)},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: user, IsBot: bot},
	}
}

func TestPassportConfigAndCallers(t *testing.T) {
	c := passCore(4, false)
	cfg, err := c.HelpGetPassportConfig(&mtproto.TLHelpGetPassportConfig{})
	if err != nil || cfg.GetPredicateName() != mtproto.Predicate_help_passportConfig {
		t.Fatal(err, cfg)
	}
	if cfg.GetCountriesLangs().GetData() == "" || cfg.GetHash() != book.ConfigHash() {
		t.Fatal(cfg)
	}
	same, err := c.HelpGetPassportConfig(&mtproto.TLHelpGetPassportConfig{Hash: cfg.GetHash()})
	if err != nil || same.GetPredicateName() != mtproto.Predicate_help_passportConfigNotModified {
		t.Fatal(err, same)
	}
	bot := passCore(9, true)
	if _, err = bot.AccountSaveSecureValue(&mtproto.TLAccountSaveSecureValue{}); err != mtproto.ErrBotMethodInvalid {
		t.Fatal(err)
	}
	if _, err = c.UsersSetSecureValueErrors(&mtproto.TLUsersSetSecureValueErrors{}); err != mtproto.ErrUserBotRequired {
		t.Fatal(err)
	}
	if _, err = bot.UsersSetSecureValueErrors(&mtproto.TLUsersSetSecureValueErrors{
		Id: mtproto.MakeTLInputUserSelf(&mtproto.InputUser{}).To_InputUser(),
	}); err != mtproto.ErrUserIdInvalid {
		t.Fatal(err)
	}
}

func TestAuthorizationFormHandler(t *testing.T) {
	c := passCore(4, false)
	loadBotHook = func(_ *PassportCore, botID int64) (*mtproto.User, string, error) {
		if botID != 90 {
			return nil, "", mtproto.ErrBotInvalid
		}
		return mtproto.MakeTLUser(&mtproto.User{
			Id:        botID,
			Bot:       true,
			FirstName: wrapperspb.String("Tellomi"),
		}).To_User(), "https://tellomi.cn/privacy", nil
	}
	t.Cleanup(func() { loadBotHook = nil })
	form, err := c.AccountGetAuthorizationForm(&mtproto.TLAccountGetAuthorizationForm{
		BotId:     90,
		Scope:     `{"v":1,"d":["pd"]}`,
		PublicKey: "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA\n-----END PUBLIC KEY-----",
	})
	if err != nil {
		t.Fatal(err)
	}
	if form.GetPredicateName() != mtproto.Predicate_account_authorizationForm {
		t.Fatal(form.GetPredicateName())
	}
	if len(form.GetUsers()) != 1 || !form.GetUsers()[0].GetBot() || form.GetPrivacyPolicyUrl().GetValue() == "" {
		t.Fatalf("%+v", form)
	}
	if len(form.GetRequiredTypes()) != 1 {
		t.Fatal(form.GetRequiredTypes())
	}
	sent, err := c.AccountSendVerifyPhoneCode(&mtproto.TLAccountSendVerifyPhoneCode{PhoneNumber: "9996612345"})
	if err != nil {
		t.Fatal(err)
	}
	if sent.GetPredicateName() != mtproto.Predicate_auth_sentCode || sent.GetType().GetPredicateName() != mtproto.Predicate_auth_sentCodeTypeSms || sent.GetType().GetLength() != 5 {
		t.Fatalf("%+v", sent)
	}
	if _, err = c.AccountVerifyPhone(&mtproto.TLAccountVerifyPhone{
		PhoneNumber:   "9996612345",
		PhoneCodeHash: sent.GetPhoneCodeHash(),
		PhoneCode:     "11111",
	}); err != nil {
		t.Fatal(err)
	}
}
