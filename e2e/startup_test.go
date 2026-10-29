package e2e

import (
	"testing"
)

func TestStartupRPCs(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	for _, profile := range []Profile{ProfileIOS, ProfileAndroid} {
		t.Run(profile.LangPack, func(t *testing.T) {
			a.SetProfile(profile)
			for _, call := range startupCalls(profile) {
				t.Run(call.Name, func(t *testing.T) {
					c, cancel := ctx()
					defer cancel()
					_, err := a.Invoke(c, call.Req)
					if err == nil {
						return
					}
					if !serverError(err) {
						// A typed 4xx is a normal refusal. 500, a timeout, or
						// a decode failure is what keeps the client on Updating.
						t.Logf("non-fatal: %v", err)
						return
					}
					t.Fatal(err)
				})
			}
		})
	}
	a.SetProfile(ProfileIOS)
}
