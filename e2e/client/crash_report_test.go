package e2e

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestCrashReportBot resolves @TellomiBot, reads the app-config key, sends a
// crash zip, and checks the object, the metadata row, the query API, and that
// media purge leaves the copy in place. The migration and the CLI ensure
// command are each run twice.
func TestCrashReportBot(t *testing.T) {
	cfg := requireE2E(t)
	before := officialUserSnapshot(t)
	runCrashSQL(t)
	runCrashSQL(t)
	if got := officialUserSnapshot(t); got != before {
		t.Fatalf("official user changed %q -> %q", before, got)
	}
	runCrashEnsure(t)
	runCrashEnsure(t)
	if n := strings.TrimSpace(mustSQL(t, `SELECT COUNT(*) FROM users WHERE id=777006 AND user_type=3 AND is_bot=1 AND username='TellomiBot'`)); n != "1" {
		t.Fatalf("bot user count %s", n)
	}
	if n := strings.TrimSpace(mustSQL(t, `SELECT COUNT(*) FROM username WHERE username='TellomiBot' AND peer_type=2 AND peer_id=777006 AND deleted=0`)); n != "1" {
		t.Fatalf("username count %s", n)
	}

	a, self := loginNamed(t, cfg, ProfileIOS, "crash_sender", "CrashSender")
	app := appConfigMap(t, invokeOK(t, a, &mtproto.TLHelpGetAppConfig61E3F854{Constructor: mtproto.CRC32_help_getAppConfig61E3F854}))
	if app["tellomi_crash_report_bot"] != "TellomiBot" {
		t.Fatalf("tellomi_crash_report_bot %#v", app["tellomi_crash_report_bot"])
	}

	c, cancel := ctx()
	resolved, err := a.Invoke(c, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_725afbbc,
		Username:    "TellomiBot",
	})
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	peer, ok := resolved.(*mtproto.TLContactsResolvedPeer)
	if !ok || peer.GetPeer().GetUserId() != 777006 {
		t.Fatalf("resolve %T peer %+v", resolved, peerUserID(resolved))
	}
	var access int64
	var sawBot bool
	for _, u := range peer.GetUsers() {
		if u.GetId() != 777006 {
			continue
		}
		access = u.GetAccessHash().GetValue()
		sawBot = u.GetBot()
		if !strings.EqualFold(u.GetUsername().GetValue(), "TellomiBot") {
			t.Fatalf("username %q", u.GetUsername().GetValue())
		}
	}
	if !sawBot || access == 0 {
		t.Fatalf("bot %v access %d", sawBot, access)
	}
	to := inputPeerUser(777006, access)

	payload := []byte("tellomi-crash-report-zip-v1")
	_, msgID := sendCrashFile(t, a, to, payload)
	row := waitCrashRow(t, self.ID, msgID)
	class, path, ok := splitCrashKey(row)
	if !ok {
		t.Fatalf("storage key %q", row)
	}
	if !strings.HasPrefix(class+"/"+path, "crash-reports/") {
		t.Fatalf("key %s/%s", class, path)
	}
	if !attachmentObjectExists(t, class, path) {
		t.Fatal("crash object was not stored")
	}

	cx, cancel := ctx()
	_, err = a.SendText(cx, to, "plain crash note ios", nil)
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*) FROM crash_reports WHERE sender_user_id=%d AND kind='text' AND message_text LIKE 'plain crash note%%'`, self.ID))); n != "1" {
		t.Fatalf("text rows %s", n)
	}

	body := crashQuery(t, fmt.Sprintf("/internal/v1/crash-reports?sender=%d&from=%d&to=%d&device=ios", self.ID, time.Now().Add(-time.Hour).Unix(), time.Now().Add(time.Hour).Unix()))
	if !strings.Contains(body, `"sender_user_id":`+fmt.Sprint(self.ID)) || !strings.Contains(body, "Tellomi-iOS-14.0.3-crash.zip") || !strings.Contains(body, `"download_url"`) {
		t.Fatalf("query body %s", body)
	}
	other := crashQuery(t, fmt.Sprintf("/internal/v1/crash-reports?sender=%d", self.ID+999999))
	if strings.Contains(other, "Tellomi-iOS-14.0.3-crash.zip") {
		t.Fatalf("sender filter returned the zip: %s", other)
	}
	future := crashQuery(t, fmt.Sprintf("/internal/v1/crash-reports?sender=%d&from=%d", self.ID, time.Now().Add(24*time.Hour).Unix()))
	if strings.Contains(future, "Tellomi-iOS-14.0.3-crash.zip") {
		t.Fatalf("time filter returned the zip: %s", future)
	}
	unauth := crashQueryStatus(t, "/internal/v1/crash-reports", "")
	if unauth == 200 {
		t.Fatal("query succeeded without a token")
	}

	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{msgID},
	})
	runPurge(t)
	if !attachmentObjectExists(t, class, path) {
		t.Fatal("media purge deleted the crash report object")
	}
}

func sendCrashFile(t *testing.T, c *Client, to *mtproto.InputPeer, payload []byte) (*mtproto.Document, int32) {
	t.Helper()
	const name = "Tellomi-iOS-14.0.3-crash.zip"
	fileID := uploadBytes(t, c, payload)
	res := invokeOK(t, c, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Message:     "app=14.0.3 device=iPhone15,2",
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File:     mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: name}).To_InputFile(),
			MimeType: "application/zip",
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: name}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	for _, m := range allMessages(res) {
		if doc := m.GetMedia().GetDocument(); doc != nil && doc.GetId() != 0 {
			return doc, m.GetId()
		}
	}
	t.Fatalf("sendMedia returned no document (%T)", res)
	return nil, 0
}

func officialUserSnapshot(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(mustSQL(t, `SELECT id, user_type, username, first_name FROM users WHERE id=777000`))
}

func runCrashSQL(t *testing.T) {
	t.Helper()
	out, err := composeExec(time.Minute, "", "bash", "-c", `mysql -hmysql -uroot -proot --default-character-set=utf8mb4 teamgram < /app/deploy/sql/migrate-20261125-crash-report-bot.sql`)
	if err != nil {
		t.Fatalf("migration: %v\n%s", err, out)
	}
}

func runCrashEnsure(t *testing.T) {
	t.Helper()
	out, err := composeExec(time.Minute, "", "/app/bin/crashreport", "ensure", "-f", "/app/etc2/bff.yaml")
	if err != nil {
		t.Fatalf("ensure: %v\n%s", err, out)
	}
	if !strings.Contains(out, "ok") {
		t.Fatalf("ensure output %s", out)
	}
}

func waitCrashRow(t *testing.T, sender int64, msgID int32) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	q := fmt.Sprintf(`SELECT CONCAT(storage_class, '|', storage_key) FROM crash_reports WHERE sender_user_id=%d AND msg_id=%d`, sender, msgID)
	var last string
	for time.Now().Before(deadline) {
		last = strings.TrimSpace(mustSQL(t, q))
		if last != "" && !strings.HasPrefix(last, "mysql:") {
			return last
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("no crash_reports row for %d/%d (%s)", sender, msgID, last)
	return ""
}

func splitCrashKey(raw string) (class, path string, ok bool) {
	raw = strings.TrimSpace(raw)
	class, key, found := strings.Cut(raw, "|")
	if !found || key == "" {
		return "", "", false
	}
	const prefix = "crash-reports/"
	if strings.HasPrefix(key, prefix) {
		return "crash-reports", strings.TrimPrefix(key, prefix), true
	}
	return class, key, class != ""
}

func appConfigMap(t *testing.T, obj mtproto.TLObject) map[string]any {
	t.Helper()
	var cfg *mtproto.JSONValue
	switch v := obj.(type) {
	case *mtproto.TLHelpAppConfig:
		if v.GetData2() != nil {
			cfg = v.GetData2().GetConfig()
		}
	case *mtproto.Help_AppConfig:
		cfg = v.GetConfig()
	default:
		t.Fatalf("help.getAppConfig %T", obj)
	}
	raw, ok := mtproto.FromJSONValue(cfg).(map[string]any)
	if !ok {
		t.Fatalf("config %T", mtproto.FromJSONValue(cfg))
	}
	return raw
}

func peerUserID(obj mtproto.TLObject) int64 {
	if v, ok := obj.(*mtproto.TLContactsResolvedPeer); ok {
		return v.GetPeer().GetUserId()
	}
	return 0
}

func crashQuery(t *testing.T, path string) string {
	t.Helper()
	code, body := crashHTTP(t, path, true)
	if code != 200 {
		t.Fatalf("%s -> %d %s", path, code, body)
	}
	return body
}

func crashQueryStatus(t *testing.T, path, auth string) int {
	t.Helper()
	code, _ := crashHTTPRaw(t, path, auth)
	return code
}

func crashHTTP(t *testing.T, path string, auth bool) (int, string) {
	t.Helper()
	header := ""
	if auth {
		header = "auth"
	}
	return crashHTTPRaw(t, path, header)
}

func crashHTTPRaw(t *testing.T, path, auth string) (int, string) {
	t.Helper()
	script := `exec 3<>/dev/tcp/127.0.0.1/8818 || exit 90
{
  printf 'GET %s HTTP/1.1\r\nHost: 127.0.0.1\r\nConnection: close\r\n' "$E2E_PATH"
  if [ "$E2E_AUTH" = "auth" ]; then printf 'Authorization: Bearer %s\r\n' "$TELLOMI_CRASH_REPORT_TOKEN"; fi
  printf '\r\n'
} >&3
cat <&3
`
	ctx, cancel := contextTimeout(20 * time.Second)
	defer cancel()
	args := append(composeBase(), "exec", "-T", "-e", "E2E_PATH="+path, "-e", "E2E_AUTH="+auth, "teamgram", "bash", "-c", script)
	cmd := exec.CommandContext(ctx, "docker", args...)
	if dir := os.Getenv("TELLOMI_E2E_ROOT"); dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("query: %v\n%s", err, out)
	}
	return splitHTTP(string(out))
}
