package core

import (
	"testing"

	"github.com/teamgram/teamgram-server/app/bff/qrcode/internal/model"
)

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
