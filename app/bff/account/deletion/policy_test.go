package deletion

import (
	"testing"
	"time"
)

func TestDecide(t *testing.T) {
	now := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	old := now.Add(-8 * 24 * time.Hour)
	recent := now.Add(-24 * time.Hour)
	cases := []struct {
		name   string
		in     Input
		action Action
		wait   int32
	}{
		{name: "no password", in: Input{Now: now}, action: DeleteNow},
		{name: "password accepted", in: Input{Now: now, HasPassword: true, PasswordProvided: true, PasswordAccepted: true, PasswordSetAt: recent}, action: DeleteNow},
		{name: "omitted and old and active", in: Input{Now: now, HasPassword: true, PasswordSetAt: old, ActiveWithin7Days: true}, action: Delay, wait: int32(Wait / time.Second)},
		{name: "unknown age counts as old", in: Input{Now: now, HasPassword: true, ActiveWithin7Days: true}, action: Delay, wait: int32(Wait / time.Second)},
		{name: "omitted but inactive", in: Input{Now: now, HasPassword: true, PasswordSetAt: old}, action: DeleteNow},
		{name: "omitted but password is recent", in: Input{Now: now, HasPassword: true, PasswordSetAt: recent, ActiveWithin7Days: true}, action: DeleteNow},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Decide(tc.in)
			if got.Action != tc.action || got.WaitSeconds != tc.wait {
				t.Fatalf("%+v", got)
			}
		})
	}
}
