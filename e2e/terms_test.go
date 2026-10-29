package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestHelpGetTermsOfServiceUpdate locks the post-#33/#35 contract.
// The default config never returns a popup, and asking again stays empty.
// When a terms update is returned, accepting that id makes the next call empty.
func TestHelpGetTermsOfServiceUpdate(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "terms", "Terms")

	first := invokeOK(t, c, &mtproto.TLHelpGetTermsOfServiceUpdate{
		Constructor: mtproto.CRC32_help_getTermsOfServiceUpdate,
	})
	switch cur := first.(type) {
	case *mtproto.TLHelpTermsOfServiceUpdateEmpty:
		assertEmptyTerms(t, cur)
		second := invokeOK(t, c, &mtproto.TLHelpGetTermsOfServiceUpdate{
			Constructor: mtproto.CRC32_help_getTermsOfServiceUpdate,
		})
		empty, ok := second.(*mtproto.TLHelpTermsOfServiceUpdateEmpty)
		if !ok {
			t.Fatalf("second getTermsOfServiceUpdate returned %T (%s); the terms popup came back", second, predicateName(second))
		}
		assertEmptyTerms(t, empty)
	case *mtproto.TLHelpTermsOfServiceUpdate:
		terms := cur.GetTermsOfService()
		if terms == nil || terms.GetId() == nil || terms.GetId().GetData() == "" {
			t.Fatal("terms of service id is empty")
		}
		accepted := invokeOK(t, c, &mtproto.TLHelpAcceptTermsOfService{
			Constructor: mtproto.CRC32_help_acceptTermsOfService,
			Id:          terms.GetId(),
		})
		if _, ok := accepted.(*mtproto.TLBoolTrue); !ok {
			t.Fatalf("help.acceptTermsOfService returned %T", accepted)
		}
		second := invokeOK(t, c, &mtproto.TLHelpGetTermsOfServiceUpdate{
			Constructor: mtproto.CRC32_help_getTermsOfServiceUpdate,
		})
		empty, ok := second.(*mtproto.TLHelpTermsOfServiceUpdateEmpty)
		if !ok || empty.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdateEmpty {
			t.Fatalf("after accept, getTermsOfServiceUpdate returned %T (%s); the terms popup will show again", second, predicateName(second))
		}
		assertEmptyTerms(t, empty)
	default:
		t.Fatalf("getTermsOfServiceUpdate returned %T", first)
	}
}

func assertEmptyTerms(t *testing.T, empty *mtproto.TLHelpTermsOfServiceUpdateEmpty) {
	t.Helper()
	if empty.GetPredicateName() != mtproto.Predicate_help_termsOfServiceUpdateEmpty {
		t.Fatalf("predicate %s", empty.GetPredicateName())
	}
	if empty.GetExpires() < int32(time.Now().Unix())+60 {
		t.Fatalf("empty expires %d; the client will ask again immediately", empty.GetExpires())
	}
}

func predicateName(obj mtproto.TLObject) string {
	type named interface{ GetPredicateName() string }
	if n, ok := obj.(named); ok {
		return n.GetPredicateName()
	}
	return ""
}
