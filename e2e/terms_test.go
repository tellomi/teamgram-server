package e2e

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestHelpGetTermsOfServiceUpdate(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "terms", "Terms")

	first := invokeOK(t, c, &mtproto.TLHelpGetTermsOfServiceUpdate{
		Constructor: mtproto.CRC32_help_getTermsOfServiceUpdate,
	})
	update, ok := first.(*mtproto.TLHelpTermsOfServiceUpdate)
	if !ok {
		t.Fatalf("first getTermsOfServiceUpdate returned %T, want help.termsOfServiceUpdate", first)
	}
	if update.GetPredicateName() == mtproto.Predicate_help_termsOfServiceUpdateEmpty {
		t.Fatal("a new account already received help.termsOfServiceUpdateEmpty")
	}
	terms := update.GetTermsOfService()
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
}

func predicateName(obj mtproto.TLObject) string {
	type named interface{ GetPredicateName() string }
	if n, ok := obj.(named); ok {
		return n.GetPredicateName()
	}
	return ""
}
