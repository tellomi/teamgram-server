package mtproto

import "testing"

func TestPremiumFollowsStoredSubscription(t *testing.T) {
	SetMembershipFunc(nil)
	t.Cleanup(func() { SetMembershipFunc(nil) })

	plain := &ImmutableUser{User: &UserData{Id: 7, Premium: false, FirstName: "A"}}
	if plain.Premium() || plain.ToSelfUser().GetPremium() || plain.ToUser(8).GetPremium() {
		t.Fatal("row with premium=false was reported as Premium")
	}
	active := &ImmutableUser{User: &UserData{Id: 7, Premium: true, FirstName: "A"}}
	if !active.Premium() || !active.ToSelfUser().GetPremium() || !active.ToUser(8).GetPremium() {
		t.Fatal("active subscription was not reported as Premium")
	}
	expired := &ImmutableUser{User: &UserData{Id: 7, Premium: true, PremiumExpireDate: MakeFlagsInt64(1), FirstName: "A"}}
	if expired.Premium() {
		t.Fatal("expired subscription was reported as Premium")
	}
	deleted := &ImmutableUser{User: &UserData{Id: 9, Deleted: true, Premium: true}}
	if deleted.Premium() || deleted.ToDeletedUser().GetPremium() {
		t.Fatal("deleted user was reported as Premium")
	}
}

func TestPremiumPolicyCanGrantMembership(t *testing.T) {
	SetMembershipFunc(func(stored bool, expireUnix, now int64) bool { return true })
	t.Cleanup(func() { SetMembershipFunc(nil) })

	user := &ImmutableUser{User: &UserData{Id: 7, Premium: false, FirstName: "A"}}
	if !user.Premium() || !user.ToSelfUser().GetPremium() {
		t.Fatal("installed policy did not grant membership")
	}
	deleted := &ImmutableUser{User: &UserData{Id: 9, Deleted: true, Premium: false}}
	if deleted.Premium() {
		t.Fatal("policy granted membership to a deleted user")
	}
}
