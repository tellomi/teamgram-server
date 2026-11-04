package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestPremiumNotRequiredToContact checks the profile-page call that used to
// return ENTERPRISE_IS_BLOCKED. Tellomi does not require premium to contact a user.
func TestPremiumNotRequiredToContact(t *testing.T) {
	a, _, aSelf, _ := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	prem, err := a.Invoke(ctx, &mtproto.TLUsersGetIsPremiumRequiredToContact{
		Constructor: mtproto.CRC32_users_getIsPremiumRequiredToContact,
		Id:          []*mtproto.InputUser{inputUser(aSelf.ID, aSelf.AccessHash)},
	})
	if err != nil {
		t.Fatal(err)
	}
	vec, ok := prem.(*tlVector)
	if !ok || vec.Count != 1 {
		t.Fatalf("premium required %T %+v", prem, prem)
	}
}
