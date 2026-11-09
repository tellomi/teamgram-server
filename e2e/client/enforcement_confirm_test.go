package e2e

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestAccountEnforcementReportConfirm covers report → fake-model substantiated
// → @tellomi asks @erel → 是 bans with a 777000 notice and an appeal, 否 bans
// nobody, and a non-admin 是 is ignored. The model is e2e/fakebailian.
func TestAccountEnforcementReportConfirm(t *testing.T) {
	cfg := requireE2E(t)
	if os.Getenv("TELLOMI_ENFORCEMENT_E2E") != "1" {
		t.Skip("set TELLOMI_ENFORCEMENT_E2E=1 on an isolated stack with the enforcement database migrated")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	admin, _ := dialPhone(t, ctx, cfg, "9996610080", "Erel")
	setUsername(t, ctx, admin, "erel")
	reporter, _ := dialPhone(t, ctx, cfg, "9996610081", "Reporter")
	yesUser, yesSelf := dialPhone(t, ctx, cfg, "9996610082", "YesTarget")
	noUser, noSelf := dialPhone(t, ctx, cfg, "9996610083", "NoTarget")
	other, _ := dialPhone(t, ctx, cfg, "9996610084", "Other")
	ignored, ignoredSelf := dialPhone(t, ctx, cfg, "9996610085", "Ignored")

	reportUser(t, ctx, reporter, yesSelf, "私聊辱骂")
	caseMsg := waitTellomi(t, ctx, admin, strconv.FormatInt(yesSelf.ID, 10))
	if !strings.Contains(caseMsg.text, "属实") && !strings.Contains(caseMsg.text, "请确认") {
		t.Fatalf("case message: %s", caseMsg.text)
	}
	sendReply(t, ctx, admin, inputPeerUser(tellomiID, caseMsg.hash), "是", caseMsg.id)
	waitTellomi(t, ctx, admin, "已封禁")
	if _, err := yesUser.SendText(ctx, inputPeerSelf(), "after-ban", nil); err == nil {
		t.Fatal("banned account sent a message")
	}

	fresh, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	defer fresh.Close()
	hash, err := sendCode(ctx, fresh, "9996610082")
	if err != nil {
		t.Fatal(err)
	}
	_, err = signIn(ctx, fresh, "9996610082", hash, "11111")
	rpc, ok := asRPCError(err)
	if !ok || rpc.Code != 401 || rpc.Message != "ACCOUNT_PERMANENT_DISABLED" {
		t.Fatalf("permanent signIn: %v", err)
	}
	claim := loopback(t, ctx, "/internal/v1/enforcement/disposition/claim", fmt.Sprintf(`{"proof":"%s"}`, hash), "")
	token := between(t, claim, `"token":"`, `"`)
	notes := loopback(t, ctx, "/internal/v1/enforcement/disposition/notices", `{}`, "Disposition "+token)
	if !strings.Contains(notes, `"from":777000`) || !strings.Contains(notes, "处置编号") {
		t.Fatalf("777000 notices: %s", notes)
	}
	sanction := between(t, notes, "处置编号：", "。")
	lifted := loopback(t, ctx, "/internal/v1/enforcement/disposition/appeals", fmt.Sprintf(`{"sanction_id":"%s","text":"e2e-lift"}`, sanction), "Disposition "+token)
	if !strings.Contains(lifted, `"status":"lifted"`) {
		t.Fatalf("appeal: %s", lifted)
	}
	restored, restoredSelf := dialPhone(t, ctx, cfg, "9996610082", "YesTarget")
	if restoredSelf.ID != yesSelf.ID {
		t.Fatalf("restored id %d want %d", restoredSelf.ID, yesSelf.ID)
	}
	if _, err = restored.SendText(ctx, inputPeerSelf(), "after-appeal", nil); err != nil {
		t.Fatalf("send after appeal lift: %v", err)
	}

	reportUser(t, ctx, reporter, noSelf, "另一条辱骂")
	noMsg := waitTellomi(t, ctx, admin, strconv.FormatInt(noSelf.ID, 10))
	sendReply(t, ctx, admin, inputPeerUser(tellomiID, noMsg.hash), "否", noMsg.id)
	waitTellomi(t, ctx, admin, "没有封禁")
	if _, err = noUser.SendText(ctx, inputPeerSelf(), "still-open", nil); err != nil {
		t.Fatalf("否 banned the account: %v", err)
	}

	reportUser(t, ctx, reporter, ignoredSelf, "第三条辱骂")
	waitTellomi(t, ctx, admin, strconv.FormatInt(ignoredSelf.ID, 10))
	tellomiPeer := resolveTellomi(t, ctx, other)
	if _, err = other.SendText(ctx, tellomiPeer, "是", nil); err != nil {
		t.Fatalf("non-admin message: %v", err)
	}
	time.Sleep(2 * time.Second)
	if _, err = ignored.SendText(ctx, inputPeerSelf(), "not-banned", nil); err != nil {
		t.Fatalf("non-admin reply banned the account: %v", err)
	}
}

const tellomiID int64 = 777001

type chatLine struct {
	id   int32
	hash int64
	text string
}

func dialPhone(t *testing.T, ctx context.Context, cfg Config, number, name string) (*Client, Self) {
	t.Helper()
	c, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	self, err := c.Login(ctx, Phone{Number: number, Code: "11111"}, name)
	if err != nil {
		t.Fatal(err)
	}
	return c, self
}

func setUsername(t *testing.T, ctx context.Context, c *Client, name string) {
	t.Helper()
	updated, err := c.Invoke(ctx, &mtproto.TLAccountUpdateUsername{
		Constructor: mtproto.CRC32_account_updateUsername,
		Username:    name,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := updated.(*mtproto.TLUser); !ok {
		t.Fatalf("account.updateUsername returned %T", updated)
	}
}

func reportUser(t *testing.T, ctx context.Context, reporter *Client, target Self, note string) {
	t.Helper()
	_, err := reporter.Invoke(ctx, &mtproto.TLAccountReportPeer{
		Constructor: mtproto.CRC32_account_reportPeer,
		Peer:        inputPeerUser(target.ID, target.AccessHash),
		Reason:      mtproto.MakeTLInputReportReasonSpam(&mtproto.ReportReason{}).To_ReportReason(),
		Message:     note,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func waitTellomi(t *testing.T, ctx context.Context, c *Client, needle string) chatLine {
	t.Helper()
	deadline := time.Now().Add(40 * time.Second)
	for time.Now().Before(deadline) {
		if line, ok := findTellomi(t, ctx, c, needle); ok {
			return line
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(time.Second):
		}
	}
	t.Fatalf("@tellomi history has no message containing %s", needle)
	return chatLine{}
}

func findTellomi(t *testing.T, ctx context.Context, c *Client, needle string) (chatLine, bool) {
	t.Helper()
	page, err := c.dialogPage(ctx)
	if err != nil {
		return chatLine{}, false
	}
	var hash int64
	found := false
	for _, user := range page.Users {
		if user != nil && user.GetId() == tellomiID {
			if user.GetAccessHash() != nil {
				hash = user.GetAccessHash().GetValue()
			}
			found = true
		}
	}
	if !found {
		return chatLine{}, false
	}
	msgs, err := c.History(ctx, inputPeerUser(tellomiID, hash), 40)
	if err != nil {
		return chatLine{}, false
	}
	for _, m := range msgs {
		if m != nil && strings.Contains(m.GetMessage(), needle) {
			return chatLine{id: m.GetId(), hash: hash, text: m.GetMessage()}, true
		}
	}
	return chatLine{}, false
}

func sendReply(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, text string, replyTo int32) {
	t.Helper()
	req := &mtproto.TLMessagesSendMessage{
		Constructor: mtproto.CRC32_messages_sendMessage_fef48f62,
		Peer:        peer,
		Message:     text,
		RandomId:    randomID(),
	}
	if replyTo != 0 {
		req.ReplyToMsgId = &wrapperspb.Int32Value{Value: replyTo}
	}
	if _, err := c.Invoke(ctx, req); err != nil {
		t.Fatal(err)
	}
}

func resolveTellomi(t *testing.T, ctx context.Context, c *Client) *mtproto.InputPeer {
	t.Helper()
	res, err := c.Invoke(ctx, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_725afbbc,
		Username:    "tellomi",
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := res.(*mtproto.TLContactsResolvedPeer)
	if !ok || resolved.GetPeer().GetUserId() != tellomiID {
		t.Fatalf("resolve @tellomi: %T", res)
	}
	var hash int64
	for _, user := range resolved.GetUsers() {
		if user != nil && user.GetId() == tellomiID && user.GetAccessHash() != nil {
			hash = user.GetAccessHash().GetValue()
		}
	}
	return inputPeerUser(tellomiID, hash)
}
