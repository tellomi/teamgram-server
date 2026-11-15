package mtproto

import "testing"

func TestOtherAdminBitIsReserved(t *testing.T) {
	rights := AdminRights(ADMIN_OTHER)
	if !rights.CanOther() {
		t.Fatal("other bit not stored")
	}
	if rights.CanChangeInfo() || rights.CanBanUsers() || rights.CanPinMessages() || rights.CanInviteUsers() {
		t.Fatal("other bit granted a probed setting")
	}
}
