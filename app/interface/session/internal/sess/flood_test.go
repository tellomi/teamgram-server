package sess

import (
	"bytes"
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/flood"
)

func TestSessionReturnsFloodWait(t *testing.T) {
	sess, _ := passwordSession(t, mtproto.AuthStateNormal, 42, 42)
	sess.sessList.cb.cb.Flood = flood.New(flood.NewMemory(), flood.Config{
		Mode: flood.ModeEnforce,
		Rules: map[string]flood.Rule{
			flood.BucketGlobal: {Burst: 2, Rate: 0},
		},
	})
	ctx := context.Background()
	req := &mtproto.TLUpdatesGetState{Constructor: mtproto.CRC32_updates_getState}
	for i := int64(1); i <= 2; i++ {
		if !sess.onRpcRequest(ctx, "gate", "203.0.113.8:443", newInboxMsg(i), req, nil) {
			t.Fatalf("call %d rejected", i)
		}
	}
	msg := newInboxMsg(3)
	if sess.onRpcRequest(ctx, "gate", "203.0.113.8:443", msg, req, nil) {
		t.Fatal("third call was not flood-waited")
	}
	out := sess.outQueue.Lookup(3)
	if out == nil || out.msg == nil || !bytes.Contains(out.msg.Body, []byte("FLOOD_WAIT_")) {
		t.Fatal("missing FLOOD_WAIT reply")
	}
}

func TestHostOnlyIgnoresForwardedLists(t *testing.T) {
	if hostOnly("203.0.113.8, 198.51.100.1") != "" {
		t.Fatal("forwarded list accepted")
	}
	if hostOnly("203.0.113.8:443") != "203.0.113.8" {
		t.Fatal("host:port")
	}
	if hostOnly("not-an-ip") != "" {
		t.Fatal("garbage accepted")
	}
	if hostOnly("203.0.113.8") != "203.0.113.8" {
		t.Fatal("bare ip")
	}
}

func TestSessionLogModeLetsTheCallThrough(t *testing.T) {
	sess, _ := passwordSession(t, mtproto.AuthStateNormal, 42, 42)
	sess.sessList.cb.cb.Flood = flood.New(flood.NewMemory(), flood.Config{
		Mode: flood.ModeLog,
		Rules: map[string]flood.Rule{
			flood.BucketGlobal: {Burst: 1, Rate: 0},
		},
	})
	ctx := context.Background()
	req := &mtproto.TLUpdatesGetState{Constructor: mtproto.CRC32_updates_getState}
	if !sess.onRpcRequest(ctx, "gate", "127.0.0.1", newInboxMsg(1), req, nil) {
		t.Fatal("first")
	}
	if !sess.onRpcRequest(ctx, "gate", "127.0.0.1", newInboxMsg(2), req, nil) {
		t.Fatal("log-only rejected the call")
	}
}

func TestSessionSendCodeIsPerPhone(t *testing.T) {
	sess, wrapper := passwordSession(t, mtproto.AuthStateUnknown, 0, 0)
	wrapper.state = mtproto.AuthStateUnknown
	sess.sessList.cb.cb.Flood = flood.New(flood.NewMemory(), flood.Config{
		Mode: flood.ModeEnforce,
		Rules: map[string]flood.Rule{
			flood.BucketGlobal:        {Burst: 100, Rate: 0},
			flood.BucketSendCodePhone: {Burst: 1, Rate: 0},
			flood.BucketSendCodeIP:    {Burst: 100, Rate: 0},
		},
	})
	ctx := context.Background()
	first := &mtproto.TLAuthSendCode{Constructor: mtproto.CRC32_auth_sendCode, PhoneNumber: "+86 138 0000 0001"}
	second := &mtproto.TLAuthSendCode{Constructor: mtproto.CRC32_auth_sendCode, PhoneNumber: "+8613800000001"}
	other := &mtproto.TLAuthSendCode{Constructor: mtproto.CRC32_auth_sendCode, PhoneNumber: "+8613800000002"}
	if !sess.onRpcRequest(ctx, "gate", "198.51.100.4", newInboxMsg(1), first, nil) {
		t.Fatal("first sendCode")
	}
	if sess.onRpcRequest(ctx, "gate", "198.51.100.4", newInboxMsg(2), second, nil) {
		t.Fatal("same phone was not limited")
	}
	if !sess.onRpcRequest(ctx, "gate", "198.51.100.4", newInboxMsg(3), other, nil) {
		t.Fatal("other phone limited")
	}
}
