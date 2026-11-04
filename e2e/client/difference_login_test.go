package e2e

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// TestLoginSessionSurvivesGetDifference is the login-safe check for
// updates.getDifference. It signs in through the shared session, asks for
// the difference at the current pts, and then calls getState again.
//
// The default server leaves TELLOMI_DIFFERENCE_STRICT unset, so this must
// not come back as differenceTooLong and must not drop the session.
func TestLoginSessionSurvivesGetDifference(t *testing.T) {
	a, _, _, _ := sharedPair(t)
	c, cancel := ctx()
	defer cancel()
	pts, qts, date, err := a.State(c)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := a.Difference(c, pts, qts, date)
	if err != nil {
		t.Fatalf("updates.getDifference: %v", err)
	}
	switch diff.(type) {
	case *mtproto.TLUpdatesDifference, *mtproto.TLUpdatesDifferenceSlice, *mtproto.TLUpdatesDifferenceEmpty:
	case *mtproto.TLUpdatesDifferenceTooLong:
		t.Fatal("differenceTooLong on a fresh state; TELLOMI_DIFFERENCE_STRICT must stay off for this suite")
	default:
		t.Fatalf("updates.getDifference returned %T", diff)
	}
	if _, _, _, err = a.State(c); err != nil {
		t.Fatalf("session after getDifference: %v", err)
	}
}
