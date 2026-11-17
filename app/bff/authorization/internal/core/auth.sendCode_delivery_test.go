package core

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/logic"
	"github.com/teamgram/teamgram-server/app/bff/authorization/internal/svc"
	"github.com/teamgram/teamgram-server/app/bff/authorization/model"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
	"github.com/teamgram/teamgram-server/pkg/verify"

	"github.com/zeromicro/go-zero/core/logx"
)

// scriptedSender records whether auth.sendCode tried to deliver an SMS.
// block waits until the RPC context ends, the way a stalled Aliyun call did.
type scriptedSender struct {
	called bool
	block  bool
}

func (s *scriptedSender) SendSmsVerifyCode(ctx context.Context, phoneNumber, code, codeHash string) (string, error) {
	s.called = true
	if s.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return code, nil
}

func (s *scriptedSender) VerifySmsCode(context.Context, string, string, string) error {
	return nil
}

func newDeliveryCore(provider string, sender *scriptedSender) *AuthorizationCore {
	ctx := context.Background()
	return &AuthorizationCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{
			AuthLogic: &logic.AuthLogic{VerifyCodeInterface: sender},
			Verify: verify.Config{SMS: verify.SMSConfig{
				Provider:   provider,
				CodeLength: 6,
			}},
		},
		Logger: logx.WithContext(ctx),
	}
}

func TestOfficialPhoneCodeKind(t *testing.T) {
	if got := officialPhoneCodeKind("", false); got != model.SentCodeTypeNone {
		t.Fatalf("default %d", got)
	}
	if got := officialPhoneCodeKind("user@tellomi.app", false); got != model.SentCodeTypeEmailCode {
		t.Fatalf("email %d", got)
	}
	if got := officialPhoneCodeKind("", true); got != model.SentCodeTypeSetUpEmailRequired {
		t.Fatalf("setup %d", got)
	}
	if got := officialPhoneCodeKind("user@tellomi.app", true); got != model.SentCodeTypeEmailCode {
		t.Fatalf("email wins over setup %d", got)
	}
}

func TestFinishSendCodeLoginEmailSkipsTestSMS(t *testing.T) {
	phonenumber.SetTestPhoneRanges(nil)
	t.Cleanup(func() { phonenumber.SetTestPhoneRanges(nil) })

	sender := &scriptedSender{}
	mail := &recordMail{}
	c := newDeliveryCore(verify.ProviderAliyun, sender)
	c.svcCtx.EmailSender = mail
	c.svcCtx.Verify.Email.Provider = verify.ProviderLog
	c.svcCtx.Verify.Email.CodeLength = 6
	c.svcCtx.LoginEmail = func(_ context.Context, userID int64) (string, error) {
		if userID == 42 {
			return "user@tellomi.app", nil
		}
		return "", nil
	}
	codeData := &model.PhoneCodeTransaction{
		State:         model.CodeStateSend,
		PhoneCode:     "00000",
		PhoneCodeHash: "hash",
	}
	user := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 42}}
	if err := c.finishSendCode("9996612345", true, user, codeData); err != nil {
		t.Fatal(err)
	}
	if sender.called {
		t.Fatal("a phone with a login email called the SMS provider")
	}
	if mail.n != 1 || mail.to != "user@tellomi.app" {
		t.Fatalf("mail %#v", mail)
	}
	if codeData.SentCodeType != model.SentCodeTypeEmailCode || codeData.PhoneCode == "11111" || codeData.EmailPattern != "u***@tellomi.app" {
		t.Fatalf("%+v", codeData)
	}
	sent := codeData.ToAuthSentCode()
	if sent.GetType().GetPredicateName() != mtproto.Predicate_auth_sentCodeTypeEmailCode || sent.GetType().GetLength() != 6 {
		t.Fatalf("%+v", sent.GetType())
	}
}

func TestFinishSendCodeSetupRequiredSkipsSMS(t *testing.T) {
	phonenumber.SetTestPhoneRanges(nil)
	t.Cleanup(func() { phonenumber.SetTestPhoneRanges(nil) })

	sender := &scriptedSender{block: true}
	c := newDeliveryCore(verify.ProviderAliyun, sender)
	c.svcCtx.Verify.Email.Provider = verify.ProviderLog
	c.svcCtx.Verify.Email.SetupRequired = true
	codeData := &model.PhoneCodeTransaction{
		State:         model.CodeStateSend,
		PhoneCode:     "123456",
		PhoneCodeHash: "hash",
	}
	if err := c.finishSendCode("9996610182", false, nil, codeData); err != nil {
		t.Fatal(err)
	}
	if sender.called {
		t.Fatal("setup-required send called the SMS provider")
	}
	if codeData.SentCodeType != model.SentCodeTypeSetUpEmailRequired || codeData.PhoneCode != "" || codeData.State != model.CodeStateSent {
		t.Fatalf("%+v", codeData)
	}
	sent := codeData.ToAuthSentCode()
	if sent.GetType().GetPredicateName() != mtproto.Predicate_auth_sentCodeTypeSetUpEmailRequired || sent.GetPhoneCodeHash() != "hash" || sent.GetTimeout() != nil {
		t.Fatalf("%+v", sent)
	}
}

func TestFinishSendCodeSetupFlagOffKeepsTestSMS(t *testing.T) {
	phonenumber.SetTestPhoneRanges(nil)
	t.Cleanup(func() { phonenumber.SetTestPhoneRanges(nil) })

	sender := &scriptedSender{block: true}
	c := newDeliveryCore(verify.ProviderAliyun, sender)
	c.svcCtx.Verify.Email.Provider = verify.ProviderLog
	c.svcCtx.Verify.Email.SetupRequired = false
	codeData := &model.PhoneCodeTransaction{
		State:         model.CodeStateSend,
		PhoneCode:     "00000",
		PhoneCodeHash: "hash",
	}
	if err := c.finishSendCode("9996610182", false, nil, codeData); err != nil {
		t.Fatal(err)
	}
	if sender.called {
		t.Fatal("99966 number called the SMS provider")
	}
	if codeData.PhoneCode != "11111" || codeData.SentCodeType != model.SentCodeTypeSms {
		t.Fatalf("%+v", codeData)
	}
}

func TestFinishSendCodeDisabledMailerIgnoresSetupFlag(t *testing.T) {
	phonenumber.SetTestPhoneRanges(nil)
	t.Cleanup(func() { phonenumber.SetTestPhoneRanges(nil) })

	sender := &scriptedSender{}
	c := newDeliveryCore(verify.ProviderFixed, sender)
	c.svcCtx.Verify.Email.Provider = ""
	c.svcCtx.Verify.Email.SetupRequired = true
	codeData := &model.PhoneCodeTransaction{
		State:         model.CodeStateSend,
		PhoneCode:     "12345",
		PhoneCodeHash: "hash",
	}
	if err := c.finishSendCode("8613800138000", false, nil, codeData); err != nil {
		t.Fatal(err)
	}
	if !sender.called || codeData.SentCodeType != model.SentCodeTypeSms {
		t.Fatalf("called %v %+v", sender.called, codeData)
	}
}

func TestFinishSendCodeLoginEmailLookupErrorDoesNotSendSMS(t *testing.T) {
	sender := &scriptedSender{}
	c := newDeliveryCore(verify.ProviderAliyun, sender)
	c.svcCtx.Verify.SMS.Countries = []string{"86"}
	c.svcCtx.LoginEmail = func(context.Context, int64) (string, error) {
		return "", mtproto.ErrInternalServerError
	}
	user := &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 7}}
	codeData := &model.PhoneCodeTransaction{State: model.CodeStateSend, PhoneCode: "123456", PhoneCodeHash: "hash"}
	err := c.finishSendCode("8613800138000", true, user, codeData)
	if sender.called {
		t.Fatal("lookup error still sent SMS")
	}
	if rpcErrorCode(err) != "INTERNAL_SERVER_ERROR" {
		t.Fatalf("error %v", err)
	}
}

func TestDeliverLoginSMSKeepsTestPhoneCode(t *testing.T) {
	phonenumber.SetTestPhoneRanges(nil)
	t.Cleanup(func() { phonenumber.SetTestPhoneRanges(nil) })
	sender := &scriptedSender{block: true}
	c := newDeliveryCore(verify.ProviderAliyun, sender)
	got, err := c.deliverLoginSMS("9996610182", "654321")
	if err != nil || got != "11111" || sender.called {
		t.Fatalf("got %q called %v err %v", got, sender.called, err)
	}
}

func TestSendCodeTestPhoneSkipsAliyun(t *testing.T) {
	phonenumber.SetTestPhoneRanges(nil)
	t.Cleanup(func() { phonenumber.SetTestPhoneRanges(nil) })

	_, digits, err := phonenumber.CheckPhoneNumberInvalid("+99966 1 2345")
	if err != nil {
		t.Fatal(err)
	}
	sender := &scriptedSender{block: true}
	c := newDeliveryCore(verify.ProviderAliyun, sender)
	codeData := &model.PhoneCodeTransaction{
		State:         model.CodeStateSend,
		PhoneCode:     "00000",
		PhoneCodeHash: "hash",
	}
	if err := c.finishSendCode(digits, false, nil, codeData); err != nil {
		t.Fatal(err)
	}
	if sender.called {
		t.Fatal("99966 number called the SMS provider")
	}
	if codeData.PhoneCode != "11111" || codeData.PhoneCodeExtraData != "11111" {
		t.Fatalf("code %q extra %q", codeData.PhoneCode, codeData.PhoneCodeExtraData)
	}
	if codeData.SentCodeType != model.SentCodeTypeSms || codeData.State != model.CodeStateSent {
		t.Fatalf("type %d state %d", codeData.SentCodeType, codeData.State)
	}
}

func TestSendCodeStuckSMSFailsFast(t *testing.T) {
	phonenumber.SetTestPhoneRanges(nil)
	t.Cleanup(func() { phonenumber.SetTestPhoneRanges(nil) })

	old := verify.SMSSendTimeout
	verify.SMSSendTimeout = 200 * time.Millisecond
	t.Cleanup(func() { verify.SMSSendTimeout = old })

	sender := &scriptedSender{block: true}
	c := newDeliveryCore(verify.ProviderAliyun, sender)
	codeData := &model.PhoneCodeTransaction{
		State:         model.CodeStateSend,
		PhoneCode:     "123456",
		PhoneCodeHash: "hash",
	}
	start := time.Now()
	err := c.finishSendCode("8613800138000", false, nil, codeData)
	elapsed := time.Since(start)
	if !sender.called {
		t.Fatal("real +86 number did not try to send SMS")
	}
	if rpcErrorCode(err) != "SMS_CODE_CREATE_FAILED" {
		t.Fatalf("error %v", err)
	}
	if elapsed > 3*time.Second {
		t.Fatalf("sendCode waited %s for a stalled SMS provider", elapsed)
	}
}

func TestSendCodeFixedProviderReturnsWithoutBlocking(t *testing.T) {
	phonenumber.SetTestPhoneRanges(nil)
	t.Cleanup(func() { phonenumber.SetTestPhoneRanges(nil) })

	sender := &scriptedSender{}
	c := newDeliveryCore(verify.ProviderFixed, sender)
	codeData := &model.PhoneCodeTransaction{
		State:         model.CodeStateSend,
		PhoneCode:     "12345",
		PhoneCodeHash: "hash",
	}
	start := time.Now()
	if err := c.finishSendCode("8613800138000", false, nil, codeData); err != nil {
		t.Fatal(err)
	}
	if !sender.called {
		t.Fatal("fixed provider skipped the local code sender")
	}
	if codeData.PhoneCodeExtraData != "12345" || codeData.State != model.CodeStateSent {
		t.Fatalf("code %+v", codeData)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("fixed provider waited %s", time.Since(start))
	}
}

func TestSendCodeRejectsNonChinaBeforeSMS(t *testing.T) {
	sender := &scriptedSender{}
	c := newDeliveryCore(verify.ProviderAliyun, sender)
	c.svcCtx.Verify.SMS.Countries = []string{"86"}
	codeData := &model.PhoneCodeTransaction{State: model.CodeStateSend, PhoneCode: "123456", PhoneCodeHash: "hash"}
	err := c.finishSendCode("14155552671", false, nil, codeData)
	if sender.called {
		t.Fatal("non-+86 number called the SMS provider")
	}
	if rpcErrorCode(err) != "PHONE_NUMBER_INVALID" {
		t.Fatalf("error %v", err)
	}
}

func TestEmailEleventhDoesNotCallSender(t *testing.T) {
	kv := &countKV{}
	lim := verify.NewLimiter(kv, verify.Limits{ResendSeconds: 60, DailyPhone: 10})
	mail := &countingMail{}
	c := newDeliveryCore(verify.ProviderFixed, &scriptedSender{})
	c.svcCtx.EmailLimiter = lim
	c.svcCtx.EmailSender = mail
	c.svcCtx.Verify.Email.Provider = verify.ProviderLog
	c.svcCtx.Verify.Email.CodeLength = 6
	ctx := context.Background()
	_ = ctx
	addr := "user@example.com"
	for i := 0; i < 10; i++ {
		clearQuotaCooldown(kv, "email", addr)
		codeData := &model.PhoneCodeTransaction{State: model.CodeStateSend, PhoneCodeHash: "hash"}
		if err := c.sendEmail(addr, codeData); err != nil {
			t.Fatalf("send %d: %v", i+1, err)
		}
	}
	if mail.n != 10 {
		t.Fatalf("sender calls %d", mail.n)
	}
	clearQuotaCooldown(kv, "email", addr)
	err := c.sendEmail(addr, &model.PhoneCodeTransaction{State: model.CodeStateSend, PhoneCodeHash: "hash"})
	if mail.n != 10 {
		t.Fatal("eleventh email called the sender")
	}
	if rpcErrorCode(err) != "PHONE_NUMBER_FLOOD" {
		t.Fatalf("error %v", err)
	}
}

func TestSMSTimeoutConsumesTheDailySlot(t *testing.T) {
	old := verify.SMSSendTimeout
	verify.SMSSendTimeout = 40 * time.Millisecond
	t.Cleanup(func() { verify.SMSSendTimeout = old })

	kv := &countKV{}
	lim := verify.NewLimiter(kv, verify.Limits{ResendSeconds: 60, DailyPhone: 1})
	sender := &scriptedSender{block: true}
	c := newDeliveryCore(verify.ProviderAliyun, sender)
	c.svcCtx.Limiter = lim
	c.svcCtx.Verify.SMS.Countries = []string{"86"}
	phone := "8613800138000"
	codeData := &model.PhoneCodeTransaction{State: model.CodeStateSend, PhoneCode: "123456", PhoneCodeHash: "hash"}
	err := c.sendSMS(phone, codeData)
	if !sender.called || rpcErrorCode(err) != "SMS_CODE_CREATE_FAILED" {
		t.Fatalf("timeout called=%v err=%v", sender.called, err)
	}
	sender.called = false
	clearQuotaCooldown(kv, "sms", phone)
	err = c.sendSMS(phone, codeData)
	if sender.called {
		t.Fatal("the send after a timed-out attempt called the provider")
	}
	if rpcErrorCode(err) != "PHONE_NUMBER_FLOOD" {
		t.Fatalf("error %v", err)
	}
}

func clearQuotaCooldown(kv *countKV, kind, target string) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if kv.m == nil {
		return
	}
	delete(kv.m, "tellomi:code:cd:"+kind+":"+target)
}

type recordMail struct {
	n    int
	to   string
	code string
}

func (m *recordMail) SendCode(_ context.Context, to, code string) error {
	m.n++
	m.to = to
	m.code = code
	return nil
}

type countingMail struct{ n int }

func (m *countingMail) SendCode(context.Context, string, string) error {
	m.n++
	return nil
}

func TestSendCodeDailyCapSkipsProvider(t *testing.T) {
	kv := &countKV{}
	lim := verify.NewLimiter(kv, verify.Limits{ResendSeconds: 60, DailyPhone: 1})
	ctx := context.Background()
	phone := "8613800138000"
	if err := lim.AllowSend(ctx, "sms", phone, ""); err != nil {
		t.Fatal(err)
	}
	delete(kv.m, "tellomi:code:cd:sms:"+phone)
	sender := &scriptedSender{}
	c := newDeliveryCore(verify.ProviderAliyun, sender)
	c.svcCtx.Limiter = lim
	c.svcCtx.Verify.SMS.Countries = []string{"86"}
	codeData := &model.PhoneCodeTransaction{State: model.CodeStateSend, PhoneCode: "123456", PhoneCodeHash: "hash"}
	err := c.finishSendCode(phone, false, nil, codeData)
	if sender.called {
		t.Fatal("over-limit send called the SMS provider")
	}
	if rpcErrorCode(err) != "PHONE_NUMBER_FLOOD" {
		t.Fatalf("error %v", err)
	}
}

type countKV struct {
	mu sync.Mutex
	m  map[string]string
}

func (m *countKV) GetCtx(_ context.Context, key string) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		return "", nil
	}
	return m.m[key], nil
}

func (m *countKV) SetexCtx(_ context.Context, key, value string, _ int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	m.m[key] = value
	return nil
}

func (m *countKV) IncrCtx(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	n := 1
	if v := m.m[key]; v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			n = parsed + 1
		}
	}
	m.m[key] = strconv.Itoa(n)
	return int64(n), nil
}

func (m *countKV) ExpireCtx(context.Context, string, int) error { return nil }
func (m *countKV) SetnxExCtx(_ context.Context, key, value string, _ int) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	if _, ok := m.m[key]; ok {
		return false, nil
	}
	m.m[key] = value
	return true, nil
}
func (m *countKV) TtlCtx(_ context.Context, key string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.m[key]; !ok {
		return -2, nil
	}
	return 60, nil
}
func (m *countKV) EvalCtx(_ context.Context, _ string, phoneKey string, args ...any) (any, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.m == nil {
		m.m = map[string]string{}
	}
	return evalMapQuota(m.m, phoneKey, args), nil
}

func (m *countKV) DecrCtx(_ context.Context, key string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.m, key)
	return 0, nil
}

func evalMapQuota(m map[string]string, phoneKey string, args []any) int64 {
	if len(args) < 6 {
		return -1
	}
	caps := [3]int{}
	for i := 0; i < 3; i++ {
		switch n := args[i].(type) {
		case int:
			caps[i] = n
		case int64:
			caps[i] = int(n)
		}
	}
	ipKey, _ := args[4].(string)
	globalKey, _ := args[5].(string)
	keys := []string{phoneKey, ipKey, globalKey}
	cur := func(key string) int {
		if key == "" {
			return 0
		}
		n, _ := strconv.Atoi(m[key])
		return n
	}
	for i := 0; i < 3; i++ {
		if caps[i] > 0 && cur(keys[i])+1 > caps[i] {
			return int64(i + 1)
		}
	}
	for i := 0; i < 3; i++ {
		if caps[i] > 0 && keys[i] != "" {
			m[keys[i]] = strconv.Itoa(cur(keys[i]) + 1)
		}
	}
	return 0
}
