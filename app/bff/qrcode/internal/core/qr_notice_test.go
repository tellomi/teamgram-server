package core

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
)

func TestIndependentQRRetryReachesResume(t *testing.T) {
	if decideQRAccept(model.QRCodeStateAccepted, false, 7, 7, 7) != qrResume {
		t.Fatal("same owner retry after accept was rejected")
	}
	if decideQRAccept(model.QRCodeStateNew, false, 7, 7, 0) != qrMark {
		t.Fatal("lost claim response did not continue")
	}
	if decideQRAccept(model.QRCodeStateNew, true, 7, 7, 0) != qrMark {
		t.Fatal("first claim did not mark")
	}
	if decideQRAccept(model.QRCodeStateAccepted, false, 7, 8, 7) != qrReject {
		t.Fatal("other user resumed")
	}
	if decideQRAccept(model.QRCodeStateAccepted, true, 43, 43, 42) != qrReject {
		t.Fatal("new claim ignored the recorded user")
	}
	if decideQRAccept(model.QRCodeStateSuccess, false, 7, 7, 7) != qrReject {
		t.Fatal("finished login was resumed")
	}
	if claimOwnerID("7:99") != 7 || claimOwnerID("8") != 8 {
		t.Fatal("owner token")
	}
}

func TestQRLoginNotifiesOnlyAFinishedFirstAccept(t *testing.T) {
	if !qrLoginNotifies(model.QRCodeStateNew, false) {
		t.Fatal("first accept without a cloud password must notify")
	}
	if qrLoginNotifies(model.QRCodeStateNew, true) {
		t.Fatal("cloud password is not a finished login")
	}
	for _, state := range []int{0, model.QRCodeStateAccepted, model.QRCodeStateSuccess, 4} {
		if qrLoginNotifies(state, false) || qrLoginNotifies(state, true) {
			t.Fatalf("state %d notified", state)
		}
	}
}
