package e2e

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestLoginNoticeFromServiceUser checks the 777000 chat after a finished
// login. The text names the initConnection device and a Beijing minute.
// A wrong code on the same synthetic number must not add another notice.
// This is a local MTProto check. It is not a handset receipt.
func TestLoginNoticeFromServiceUser(t *testing.T) {
	cfg := requireE2E(t)
	phone, err := cfg.Phone("notice")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	ios, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ios.Close)
	iosAt := time.Now()
	if _, err = ios.Login(ctx, phone, "Notice"); err != nil {
		t.Fatal(err)
	}
	iosText := waitSecurityNotice(t, ctx, ios, ProfileIOS.DeviceModel, iosAt)
	assertSecurityNotice(t, iosText, ProfileIOS.DeviceModel, iosAt, time.Now())

	android, err := Dial(ctx, cfg, ProfileAndroid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(android.Close)
	androidAt := time.Now()
	if _, err = android.Login(ctx, phone, "Notice"); err != nil {
		t.Fatal(err)
	}
	androidText := waitSecurityNotice(t, ctx, android, ProfileAndroid.DeviceModel, androidAt)
	assertSecurityNotice(t, androidText, ProfileAndroid.DeviceModel, androidAt, time.Now())
	also := waitSecurityNotice(t, ctx, ios, ProfileAndroid.DeviceModel, androidAt)
	assertSecurityNotice(t, also, ProfileAndroid.DeviceModel, androidAt, time.Now())
	if _, _, _, err = ios.State(ctx); err != nil {
		t.Fatalf("older session stopped answering after the new login: %v", err)
	}

	before := countSecurityNotices(t, ctx, android)
	failed, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(failed.Close)
	if err = signInWrongCode(ctx, failed, phone.Number); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(8 * time.Second)
	for time.Now().Before(deadline) {
		if got := countSecurityNotices(t, ctx, android); got != before {
			t.Fatalf("security notices %d to %d after a rejected code", before, got)
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
}

func waitSecurityNotice(t *testing.T, ctx context.Context, c *Client, device string, after time.Time) string {
	t.Helper()
	deadline, ok := ctx.Deadline()
	if !ok || time.Until(deadline) > 25*time.Second {
		deadline = time.Now().Add(25 * time.Second)
	}
	for time.Now().Before(deadline) {
		msgs, err := serviceHistory(ctx, c)
		if err == nil {
			for _, m := range msgs {
				text := m.GetMessage()
				if !strings.Contains(text, "New login alert") || !strings.Contains(text, device) {
					continue
				}
				// History keeps older notices for this synthetic number.
				// A message dated before this login is not this attempt.
				if m.GetDate() != 0 && int64(m.GetDate())+30 < after.Unix() {
					continue
				}
				if from := m.GetFromId(); from != nil && from.GetUserId() != 0 && from.GetUserId() != 777000 {
					t.Fatalf("notice from user %d", from.GetUserId())
				}
				return text
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
	t.Fatalf("no 777000 notice for %s after %s", device, after.Format(time.RFC3339))
	return ""
}

func serviceHistory(ctx context.Context, c *Client) ([]*mtproto.Message, error) {
	peer, err := serviceUserPeer(ctx, c)
	if err != nil {
		return nil, err
	}
	return c.History(ctx, peer, 30)
}

func serviceUserPeer(ctx context.Context, c *Client) (*mtproto.InputPeer, error) {
	page, err := c.dialogPage(ctx)
	if err != nil {
		return nil, err
	}
	for _, user := range page.Users {
		if user == nil || user.GetId() != 777000 {
			continue
		}
		hash := int64(0)
		if user.GetAccessHash() != nil {
			hash = user.GetAccessHash().GetValue()
		}
		return inputPeerUser(777000, hash), nil
	}
	res, err := c.Invoke(ctx, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_725afbbc,
		Username:    "admin",
	})
	if err != nil {
		return nil, err
	}
	resolved, ok := res.(*mtproto.TLContactsResolvedPeer)
	if !ok {
		return nil, fmt.Errorf("contacts.resolveUsername returned %T", res)
	}
	for _, user := range resolved.GetUsers() {
		if user == nil || user.GetId() != 777000 {
			continue
		}
		hash := int64(0)
		if user.GetAccessHash() != nil {
			hash = user.GetAccessHash().GetValue()
		}
		return inputPeerUser(777000, hash), nil
	}
	return nil, fmt.Errorf("service user 777000 is not in dialogs")
}

func countSecurityNotices(t *testing.T, ctx context.Context, c *Client) int {
	t.Helper()
	msgs, err := serviceHistory(ctx, c)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, m := range msgs {
		if m != nil && strings.Contains(m.GetMessage(), "New login alert") {
			n++
		}
	}
	return n
}

func assertSecurityNotice(t *testing.T, text, device string, from, to time.Time) {
	t.Helper()
	if !strings.Contains(text, "New login alert") || !strings.Contains(text, "Beijing time") {
		t.Fatal("notice is missing the title or Beijing label")
	}
	if !strings.Contains(text, device) {
		t.Fatalf("notice does not name %s", device)
	}
	if !noticeHasBeijingStamp(text, from, to) {
		t.Fatal("notice clock is outside the login minute in Beijing")
	}
	lower := strings.ToLower(text)
	for _, banned := range []string{"验证码", "11111", "00000", "token", "auth_key"} {
		if strings.Contains(lower, banned) {
			t.Fatalf("notice contains %q", banned)
		}
	}
}

func noticeHasBeijingStamp(text string, from, to time.Time) bool {
	loc := time.FixedZone("CST", 8*3600)
	start := from.In(loc).Add(-time.Minute).Truncate(time.Minute)
	end := to.In(loc).Add(time.Minute).Truncate(time.Minute)
	for stamp := start; !stamp.After(end); stamp = stamp.Add(time.Minute) {
		if strings.Contains(text, stamp.Format("2006-01-02 15:04")) {
			return true
		}
	}
	return false
}

func signInWrongCode(ctx context.Context, c *Client, number string) error {
	sent, err := c.Invoke(ctx, &mtproto.TLAuthSendCode{
		Constructor: mtproto.CRC32_auth_sendCode,
		PhoneNumber: number,
		ApiId:       c.cfg.APIID,
		ApiHash:     c.cfg.APIHash,
		Settings:    mtproto.MakeTLCodeSettings(&mtproto.CodeSettings{}).To_CodeSettings(),
	})
	if err != nil {
		return err
	}
	code, ok := sent.(*mtproto.TLAuthSentCode)
	if !ok || code.GetPhoneCodeHash() == "" {
		return fmt.Errorf("auth.sendCode returned %T", sent)
	}
	_, err = c.Invoke(ctx, &mtproto.TLAuthSignIn{
		Constructor:          mtproto.CRC32_auth_signIn_8d52a951,
		PhoneNumber:          number,
		PhoneCodeHash:        code.GetPhoneCodeHash(),
		PhoneCode_FLAGSTRING: strFlag("00000"),
	})
	if err == nil {
		return errWrongCodeAccepted
	}
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "PHONE_CODE_INVALID" {
		return err
	}
	return nil
}

var errWrongCodeAccepted = errString("rejected code was accepted")

type errString string

func (e errString) Error() string { return string(e) }
