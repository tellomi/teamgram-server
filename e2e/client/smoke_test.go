package e2e

import (
	"fmt"
	"strings"
	"testing"
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
			if _, err := encodeTL(req); err != nil {
				failed = append(failed, m.Name+": "+err.Error())
			}
		}()
	}
	if len(failed) > 0 {
		t.Fatalf("%d methods did not encode:\n%s", len(failed), strings.Join(failed, "\n"))
	}
}

// TestZZImplementedSmoke calls every implemented P0/P1 method once.
// A 4xx or a 500 from dummy parameters is allowed. RPC_NOT_IMPLEMENTED and a
// decode error are not: those are the failures that leave a client on Updating.
// The name sorts after the other scenarios so a logout inside this test does
// not drop the shared session those tests use.
func TestZZImplementedSmoke(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	a.SetProfile(ProfileIOS)
	for _, m := range smokeMethods {
		t.Run(m.Name, func(t *testing.T) {
			req, err := smokeRequest(m)
			if err != nil {
				t.Fatal(err)
			}
			c, cancel := ctx()
			defer cancel()
			_, err = a.Invoke(c, req)
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
}
