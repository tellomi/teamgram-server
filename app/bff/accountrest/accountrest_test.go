package accountrest

import (
	"encoding/json"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func mustEncode(t *testing.T, obj interface {
	Encode(*mtproto.EncodeBuf, int32) error
}) {
	t.Helper()
	if err := obj.Encode(mtproto.NewEncodeBuf(512), 232); err != nil {
		t.Fatal(err)
	}
}

func codeOf(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func registerCredential(id, challenge, origin string) *mtproto.InputPasskeyCredential {
	raw, _ := json.Marshal(map[string]string{
		"type":      "webauthn.create",
		"challenge": challenge,
		"origin":    origin,
	})
	return mtproto.MakeTLInputPasskeyCredentialPublicKey(&mtproto.InputPasskeyCredential{
		Id:    id,
		RawId: id,
		Response: mtproto.MakeTLInputPasskeyResponseRegister(&mtproto.InputPasskeyResponse{
			ClientData:      mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: string(raw)}).To_DataJSON(),
			AttestationData: []byte{1, 2, 3, 4},
		}).To_InputPasskeyResponse(),
	}).To_InputPasskeyCredential()
}

func loginCredential(id string) *mtproto.InputPasskeyCredential {
	return mtproto.MakeTLInputPasskeyCredentialPublicKey(&mtproto.InputPasskeyCredential{
		Id:    id,
		RawId: id,
		Response: mtproto.MakeTLInputPasskeyResponseLogin(&mtproto.InputPasskeyResponse{
			ClientData:        mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: "{}"}).To_DataJSON(),
			AuthenticatorData: []byte{1},
			Signature:         []byte{2},
			UserHandle:        "2:7",
		}).To_InputPasskeyResponse(),
	}).To_InputPasskeyCredential()
}

func TestPasskeys(t *testing.T) {
	if _, err := ListPasskeys(0); codeOf(err) != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
	if _, err := RegisterPasskey(0, nil); codeOf(err) != "AUTH_KEY_UNREGISTERED" {
		t.Fatal(err)
	}
	if _, err := RegisterPasskey(7, registerCredential("cred", "other", "https://www.tellomi.cn")); codeOf(err) != "CREDENTIAL_INVALID" {
		t.Fatal(err)
	}
	if _, err := VerifyPasskeyLogin(nil); codeOf(err) != "CREDENTIAL_INVALID" {
		t.Fatal(err)
	}
	if _, err := VerifyPasskeyLogin(loginCredential("missing")); codeOf(err) != "CREDENTIAL_INVALID" {
		t.Fatal(err)
	}
	login, err := InitPasskeyLogin(0, "")
	if login != nil || codeOf(err) != "API_ID_INVALID" {
		t.Fatal(err)
	}
}

func TestWebBrowserSettings(t *testing.T) {
	Reset()
	got, err := GetWebBrowserSettings(3, 0)
	if err != nil || got.GetPredicateName() != mtproto.Predicate_account_webBrowserSettings || got.GetHash() == 0 {
		t.Fatalf("get: %v %#v", err, got)
	}
	mustEncode(t, got)
	same, err := GetWebBrowserSettings(3, got.GetHash())
	if err != nil || same.GetPredicateName() != mtproto.Predicate_account_webBrowserSettingsNotModified {
		t.Fatalf("not modified: %v %s", err, same.GetPredicateName())
	}
	mustEncode(t, same)
	updated, err := UpdateWebBrowserSettings(3, true, false)
	if err != nil || !updated.GetOpenExternalBrowser() || updated.GetDisplayCloseButton() {
		t.Fatalf("update: %v %#v", err, updated)
	}
	mustEncode(t, updated)
	if _, err := ToggleWebBrowserException(3, "not a url", false, nil); codeOf(err) != "URL_INVALID" {
		t.Fatal(err)
	}
	ups, err := ToggleWebBrowserException(3, "https://Example.com/path", false, mtproto.BoolTrue)
	if err != nil || ups.GetPredicateName() != mtproto.Predicate_updates || len(ups.GetUpdates()) != 1 {
		t.Fatalf("toggle: %v %#v", err, ups)
	}
	mustEncode(t, ups)
	listed, err := GetWebBrowserSettings(3, 0)
	if err != nil || len(listed.GetExternalExceptions()) != 1 || listed.GetExternalExceptions()[0].GetDomain() != "example.com" {
		t.Fatalf("external: %#v", listed)
	}
	if _, err := ToggleWebBrowserException(3, "https://example.com", true, nil); err != nil {
		t.Fatal(err)
	}
	cleared, err := DeleteWebBrowserExceptions(3)
	if err != nil || len(cleared.GetExternalExceptions()) != 0 || len(cleared.GetInappExceptions()) != 0 || !cleared.GetOpenExternalBrowser() {
		t.Fatalf("clear: %v %#v", err, cleared)
	}
	mustEncode(t, cleared)
}

func TestTakeout(t *testing.T) {
	Reset()
	if _, err := FinishTakeoutSession(9, 0, true); codeOf(err) != "TAKEOUT_REQUIRED" {
		t.Fatal(err)
	}
	opened, err := InitTakeoutSession(9, &mtproto.TLAccountInitTakeoutSession{
		Contacts:              true,
		Files:                 true,
		FileMaxSize_FLAGINT64: wrapperspb.Int64(1024),
	})
	if err != nil || opened.GetPredicateName() != mtproto.Predicate_account_takeout || opened.GetId() == 0 {
		t.Fatalf("init: %v %#v", err, opened)
	}
	mustEncode(t, opened)
	if err := RequireContactsExport(9, 0); codeOf(err) != "TAKEOUT_REQUIRED" {
		t.Fatal(err)
	}
	if err := RequireContactsExport(9, opened.GetId()); err != nil {
		t.Fatal(err)
	}
	if err := RequireContactsExport(9, opened.GetId()+1); codeOf(err) != "TAKEOUT_INVALID" {
		t.Fatal(err)
	}
	messagesOnly, err := InitTakeoutSession(9, &mtproto.TLAccountInitTakeoutSession{MessageUsers: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := RequireContactsExport(9, messagesOnly.GetId()); codeOf(err) != "TAKEOUT_REQUIRED" {
		t.Fatal(err)
	}
	opened, err = InitTakeoutSession(9, &mtproto.TLAccountInitTakeoutSession{Contacts: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := FinishTakeoutSession(9, opened.GetId()+1, true); codeOf(err) != "TAKEOUT_INVALID" {
		t.Fatal(err)
	}
	ok, err := FinishTakeoutSession(9, opened.GetId(), true)
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err)
	}
	if _, err := FinishTakeoutSession(9, 0, false); codeOf(err) != "TAKEOUT_REQUIRED" {
		t.Fatal(err)
	}
}

func TestUsernames(t *testing.T) {
	Reset()
	RememberPrimary(4, "alice")
	if _, err := ToggleUsername(4, "bob", false); codeOf(err) != "USERNAME_INVALID" {
		t.Fatal(err)
	}
	if _, err := ToggleUsername(4, "alice", true); codeOf(err) != "USERNAME_NOT_MODIFIED" {
		t.Fatal(err)
	}
	ok, err := ToggleUsername(4, "alice", false)
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err)
	}
	if _, err := ReorderUsernames(4, []string{"alice"}); codeOf(err) != "ORDER_INVALID" {
		t.Fatal(err)
	}
	if _, err := ToggleUsername(4, "alice", true); err != nil {
		t.Fatal(err)
	}
	if _, err := ReorderUsernames(4, []string{"alice"}); codeOf(err) != "USERNAME_NOT_MODIFIED" {
		t.Fatal(err)
	}
	addCollectible(4, "carol")
	reordered, err := ReorderUsernames(4, []string{"carol", "alice"})
	if err != nil || !mtproto.FromBool(reordered) {
		t.Fatal(err)
	}
	mustEncode(t, reordered)
}

func addCollectible(userID int64, name string) {
	usernames.mu.Lock()
	usernames.users[userID] = append(usernames.users[userID], usernameRec{name: name, active: true})
	usernames.mu.Unlock()
}

func TestPaidMessagesAndRequirements(t *testing.T) {
	Reset()
	user := mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 8, AccessHash: 1}).To_InputUser()
	channel := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 3}).To_InputPeer()
	rev, err := GetPaidMessagesRevenue(2, nil, user)
	if err != nil || rev.GetPredicateName() != mtproto.Predicate_account_paidMessagesRevenue || rev.GetStarsAmount() != 0 {
		t.Fatalf("revenue: %v %#v", err, rev)
	}
	mustEncode(t, rev)
	if _, err := GetPaidMessagesRevenue(2, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 1}).To_InputPeer(), user); codeOf(err) != "PARENT_PEER_INVALID" {
		t.Fatal(err)
	}
	if _, err := GetPaidMessagesRevenue(2, nil, nil); codeOf(err) != "USER_ID_INVALID" {
		t.Fatal(err)
	}
	if _, err := ToggleNoPaidMessagesException(2, nil, user, false, true); codeOf(err) != "UNSUPPORTED" {
		t.Fatal(err)
	}
	ok, err := ToggleNoPaidMessagesException(2, channel, user, true, false)
	if err != nil || !mtproto.FromBool(ok) {
		t.Fatal(err)
	}
	mustEncode(t, ok)
	reqs := RequirementsToContact([]*mtproto.InputUser{user, nil})
	if len(reqs.Datas) != 2 || reqs.Datas[0].GetPredicateName() != mtproto.Predicate_requirementToContactEmpty {
		t.Fatalf("requirements: %#v", reqs.Datas)
	}
	mustEncode(t, reqs)
}
