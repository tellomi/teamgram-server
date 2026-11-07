package core

import (
	"context"
	"strconv"
	"sync"
	"testing"
	"time"

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
