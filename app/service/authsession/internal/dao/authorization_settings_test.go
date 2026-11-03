package dao

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/authsession/internal/dal/dataobject"
)

func TestPickAuthorizationSettings(t *testing.T) {
	rows := []dataobject.AuthUsersDO{
		{Id: 1, AuthKeyId: 10, Hash: 0},
		{Id: 2, AuthKeyId: 20, Hash: 99},
		{Id: 3, AuthKeyId: 30, Hash: 77},
	}

	got, ok := pickAuthorizationSettings(rows, 20, 0)
	if !ok || got.Id != 2 {
		t.Fatalf("hash 0 current = %+v ok=%v", got, ok)
	}
	got, ok = pickAuthorizationSettings(rows, 10, 0)
	if !ok || got.Id != 1 {
		t.Fatalf("hash 0 must match the caller even when the stored hash is 0, got %+v ok=%v", got, ok)
	}
	got, ok = pickAuthorizationSettings(rows, 10, 77)
	if !ok || got.Id != 3 {
		t.Fatalf("hash match = %+v ok=%v", got, ok)
	}
	if _, ok = pickAuthorizationSettings(rows, 10, 123); ok {
		t.Fatal("unknown hash matched")
	}
	if _, ok = pickAuthorizationSettings(rows, 0, 0); ok {
		t.Fatal("hash 0 without a current key matched")
	}

	dup := append(rows, dataobject.AuthUsersDO{Id: 4, AuthKeyId: 40, Hash: 77})
	if _, ok = pickAuthorizationSettings(dup, 10, 77); ok {
		t.Fatal("duplicate hash matched")
	}
}

func TestApplyAuthorizationSettings(t *testing.T) {
	row := dataobject.AuthUsersDO{
		EncryptedRequestsDisabled: true,
		CallRequestsDisabled:      false,
		Unconfirmed:               true,
	}
	next := applyAuthorizationSettings(row, authorizationSettingsPatch{})
	if authorizationSettingsChanged(row, next) || !next.Unconfirmed || !next.EncryptedRequestsDisabled {
		t.Fatalf("empty patch changed the row: %+v", next)
	}

	next = applyAuthorizationSettings(row, authorizationSettingsPatch{Confirmed: true, SetCall: true, CallRequestsDisabled: true})
	if next.Unconfirmed || !next.EncryptedRequestsDisabled || !next.CallRequestsDisabled {
		t.Fatalf("confirmed/calls = %+v", next)
	}
	if !authorizationSettingsChanged(row, next) {
		t.Fatal("patch was a no-op")
	}

	cleared := applyAuthorizationSettings(next, authorizationSettingsPatch{SetEncrypted: true, EncryptedRequestsDisabled: false})
	if cleared.EncryptedRequestsDisabled || !cleared.CallRequestsDisabled || cleared.Unconfirmed {
		t.Fatalf("clear encrypted = %+v", cleared)
	}
}

func TestAuthorizationStillUnconfirmed(t *testing.T) {
	const created int64 = 1_700_000_000
	if authorizationStillUnconfirmed(false, created, created+AuthorizationAutoconfirmPeriod+1) {
		t.Fatal("a confirmed row became unconfirmed")
	}
	if !authorizationStillUnconfirmed(true, created, created+AuthorizationAutoconfirmPeriod-1) {
		t.Fatal("a fresh unconfirmed row was auto-confirmed")
	}
	if authorizationStillUnconfirmed(true, created, created+AuthorizationAutoconfirmPeriod) {
		t.Fatal("row stayed unconfirmed after the autoconfirm period")
	}
	if !authorizationStillUnconfirmed(true, 0, time.Now().Unix()) {
		t.Fatal("missing date_created was auto-confirmed")
	}
	if AuthorizationAutoconfirmPeriod != 7*24*60*60 {
		t.Fatalf("period = %d", AuthorizationAutoconfirmPeriod)
	}
}

func TestNewSessionStartsUnconfirmed(t *testing.T) {
	if newSessionStartsUnconfirmed(0) || newSessionStartsUnconfirmed(1) {
		t.Fatal("the first live session must be confirmed")
	}
	if !newSessionStartsUnconfirmed(2) {
		t.Fatal("a second live session must start unconfirmed")
	}
}

func TestApplyStoredAuthorizationFlags(t *testing.T) {
	auth := mtproto.MakeTLAuthorization(&mtproto.Authorization{}).To_Authorization()
	applyStoredAuthorizationFlags(auth, dataobject.AuthUsersDO{
		EncryptedRequestsDisabled: true,
		CallRequestsDisabled:      true,
		Unconfirmed:               true,
		DateCreated:               time.Now().Unix(),
	}, time.Now().Unix())
	if !auth.GetEncryptedRequestsDisabled() || !auth.GetCallRequestsDisabled() || !auth.GetUnconfirmed() {
		t.Fatalf("flags = enc %v call %v unconfirmed %v", auth.GetEncryptedRequestsDisabled(), auth.GetCallRequestsDisabled(), auth.GetUnconfirmed())
	}
	applyStoredAuthorizationFlags(nil, dataobject.AuthUsersDO{Unconfirmed: true}, time.Now().Unix())
}
