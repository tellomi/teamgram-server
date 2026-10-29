package mtproto

import "testing"

func TestEveryUserIsPremium(t *testing.T) {
	user := &ImmutableUser{User: &UserData{Id: 7, Premium: false, FirstName: "A"}}
	if !user.Premium() {
		t.Fatal("non-premium row was not reported as Premium")
	}
	self := user.ToSelfUser()
	if !self.GetPremium() {
		t.Fatal("self user.premium is false")
	}
	other := user.ToUser(8)
	if other.GetSelf() || !other.GetPremium() {
		t.Fatalf("other user self=%v premium=%v", other.GetSelf(), other.GetPremium())
	}
	unsafe := user.ToUnsafeUser(&ImmutableUser{User: &UserData{Id: 8, FirstName: "B"}})
	if !unsafe.GetPremium() {
		t.Fatal("unsafe user.premium is false")
	}

	deleted := &ImmutableUser{User: &UserData{Id: 9, Deleted: true, Premium: true}}
	if deleted.Premium() || deleted.ToDeletedUser().GetPremium() {
		t.Fatal("deleted user was reported as Premium")
	}
}
