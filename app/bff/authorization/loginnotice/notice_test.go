package loginnotice

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

func TestClaimIsIdempotentPerAuthKey(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	now := time.Unix(1_700_000_000, 0)
	if Claim(777000, 1, now) || Claim(0, 1, now) {
		t.Fatal("service user or missing user")
	}
	if !Claim(9, 100, now) {
		t.Fatal("first login")
	}
	if Claim(9, 100, now.Add(30*time.Second)) {
		t.Fatal("same auth key inside two minutes")
	}
	if !Claim(9, 200, now.Add(30*time.Second)) {
		t.Fatal("a different auth key is another device")
	}
	if !Claim(9, 100, now.Add(3*time.Minute)) {
		t.Fatal("same key after the dedup window")
	}
}

func TestTextUsesDeviceAndBeijingTime(t *testing.T) {
	when := time.Date(2026, 11, 7, 2, 30, 0, 0, time.UTC)
	text, entities := Text(Device(" iPhone 16 ", "ios", "android"), when)
	if !strings.Contains(text, "新设备登录提醒") || !strings.Contains(text, "iPhone 16") {
		t.Fatalf("device: %q", text)
	}
	if !strings.Contains(text, "2026-11-07 10:30") || !strings.Contains(text, "北京时间") {
		t.Fatalf("clock: %q", text)
	}
	if strings.Contains(text, "ios") || strings.Contains(text, "android") {
		t.Fatalf("lang pack leaked into the device: %q", text)
	}
	for _, banned := range []string{"验证码", "token", "auth_key", "12345"} {
		if strings.Contains(strings.ToLower(text), banned) {
			t.Fatalf("notice contains %q", banned)
		}
	}
	if len(entities) == 0 {
		t.Fatal("title is not marked")
	}
	empty, _ := Text(Device(" ", "", ""), when)
	if !strings.Contains(empty, "未知设备") {
		t.Fatalf("empty device: %q", empty)
	}
}

func TestSendOnceFromServiceUser(t *testing.T) {
	Reset()
	t.Cleanup(Reset)
	fake := &fakePush{}
	when := time.Date(2026, 11, 8, 1, 2, 0, 0, time.UTC)
	if !Claim(9, 50, when) {
		t.Fatal("claim")
	}
	// Claim already recorded this key. Send itself does not claim again;
	// Push is what claims. Call Send directly to inspect one body.
	Reset()
	Send(context.Background(), fake, 9, "zh-hans", "Pixel 9", when)
	if fake.n != 1 {
		t.Fatalf("sends = %d", fake.n)
	}
	if fake.user != 777000 || fake.peer != 9 {
		t.Fatalf("from %d to %d", fake.user, fake.peer)
	}
	if !strings.Contains(fake.text, "Pixel 9") || strings.Contains(fake.text, "验证码") {
		t.Fatalf("body")
	}
	Send(context.Background(), nil, 9, "zh-hans", "Pixel 9", when)
	Send(context.Background(), fake, 777000, "zh-hans", "Pixel 9", when)
	if fake.n != 1 {
		t.Fatal("nil pusher or service user sent")
	}
}

type fakePush struct {
	n    int
	user int64
	peer int64
	text string
}

func (f *fakePush) MsgPushUserMessage(_ context.Context, in *msgpb.TLMsgPushUserMessage) (*mtproto.Bool, error) {
	f.n++
	if in != nil {
		f.user = in.UserId
		f.peer = in.PeerId
		if in.Message != nil && in.Message.Message != nil {
			f.text = in.Message.Message.Message
		}
	}
	return mtproto.BoolTrue, nil
}
