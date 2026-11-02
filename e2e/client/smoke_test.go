package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSmokeRequestsEncode checks that every implemented P0/P1 method has a
// request the client can encode. It does not need a server. A new endpoint
// shows up here after e2e/client/smoke_methods.go is regenerated.
func TestSmokeRequestsEncode(t *testing.T) {
	if len(smokeMethods) < 400 {
		t.Fatalf("smoke list has %d methods", len(smokeMethods))
	}
	var failed []string
	for _, m := range smokeMethods {
		req, err := smokeRequest(m)
		if err != nil {
			failed = append(failed, m.Name+": "+err.Error())
			continue
		}
		func() {
			defer func() {
				if r := recover(); r != nil {
					failed = append(failed, fmt.Sprintf("%s: panic %v", m.Name, r))
				}
			}()
			raw, err := encodeTL(req)
			if err != nil {
				failed = append(failed, m.Name+": "+err.Error())
				return
			}
			if err := smokeRoundTrip(raw); err != nil {
				failed = append(failed, m.Name+": "+err.Error())
			}
		}()
	}
	if len(failed) > 0 {
		t.Fatalf("%d methods did not encode:\n%s", len(failed), strings.Join(failed, "\n"))
	}
}

// smokeRoundTrip decodes a request the way the session service does. A flag
// that is set without a constructor makes Object() fail, and the session then
// drops the RPC without a reply.
func smokeRoundTrip(raw []byte) error {
	if err := smokeDecodeExact(raw); err != nil {
		return err
	}
	cfg := Config{APIID: 10840, APIHash: "0123456789abcdef0123456789abcdef", DC: 1}
	wrapped, err := wrapInit(cfg, ProfileIOS, raw)
	if err != nil {
		return err
	}
	obj, err := decodeTL(wrapped)
	if err != nil {
		return fmt.Errorf("invokeWithLayer: %w", err)
	}
	inv, ok := obj.(*mtproto.TLInvokeWithLayer)
	if !ok {
		return fmt.Errorf("invokeWithLayer decoded as %T", obj)
	}
	inner, err := decodeTL(inv.GetQuery())
	if err != nil {
		return fmt.Errorf("initConnection: %w", err)
	}
	init, ok := inner.(*mtproto.TLInitConnection)
	if !ok {
		return fmt.Errorf("initConnection decoded as %T", inner)
	}
	if err := smokeDecodeExact(init.GetQuery()); err != nil {
		return fmt.Errorf("query: %w", err)
	}
	return nil
}

// TestZZImplementedSmoke calls every implemented P0/P1 method once.
// A 4xx or a 500 from dummy parameters is allowed. RPC_NOT_IMPLEMENTED and a
// decode error are not: those are the failures that leave a client on Updating.
// It logs in as its own test phone. account.deleteAccount and auth.logOut run
// last so they do not drop the session the earlier calls still need, and they
// do not touch the shared Alice/Bob pair.
func TestZZImplementedSmoke(t *testing.T) {
	cfg := requireE2E(t)
	loginCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, err := Dial(loginCtx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	phone := Phone{Number: "9996610009", Code: TestPhoneCode("9996610009")}
	if _, err = c.Login(loginCtx, phone, "Smoke"); err != nil {
		t.Fatal(err)
	}

	call := func(m smokeMethod) {
		t.Helper()
		t.Run(m.Name, func(t *testing.T) {
			req, err := smokeRequest(m)
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_, err = c.Invoke(ctx, req)
			if err == nil {
				return
			}
			if rpc, ok := asRPCError(err); ok {
				msg := strings.ToUpper(rpc.Message)
				if strings.Contains(msg, "NOT_IMPL") || strings.Contains(msg, "UNIMPLEMENTED") {
					t.Fatalf("not implemented: %v", err)
				}
				return
			}
			t.Fatalf("decode or transport: %v", err)
		})
	}

	var tail []smokeMethod
	for _, m := range smokeMethods {
		if m.Name == "account.deleteAccount" || m.Name == "auth.logOut" {
			tail = append(tail, m)
			continue
		}
		call(m)
	}
	for _, m := range tail {
		call(m)
	}
}
