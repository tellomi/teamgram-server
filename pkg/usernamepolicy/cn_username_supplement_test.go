package usernamepolicy

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestCnUsernameSupplement(t *testing.T) {
	curated, gap := CnUsernameSupplementStats()
	if curated != 188 || gap != 2 {
		t.Fatalf("supplement curated=%d gap=%d, want 188 and 2", curated, gap)
	}
	e := runtimeEngine()
	cn := []string{"global", "cn"}
	global := []string{"global"}
	denied := []string{
		"gongchandang", "GongChanDang", "GONGCHANDANG",
		"ｇｏｎｇｃｈａｎｄａｎｇ",
		"gongchandang88", "88gongchandang", "gongchandang_fan", "my_gongchandang",
		"g0ngchandang", "g0ngchandang88",
		"xijinping", "XiJinPing", "XIJINPING",
		"ｘｉｊｉｎｐｉｎｇ",
		"xijinping_01", "01_xijinping", "xijinping88",
		"x1j1np1ng", "x1jinping88",
		"chengkejie88",
	}
	for _, name := range denied {
		for _, field := range []string{FieldUsername, FieldSlug, FieldBotUsername, FieldDisplayName} {
			v := e.Check(name, field, cn)
			if v.IsAllowed() {
				t.Fatalf("%s %s was allowed", field, name)
			}
			if v.Outcome != OutcomeRegionalRestricted || !strings.HasPrefix(v.RuleID, "supplement/cn-username:") {
				t.Fatalf("%s %s verdict %+v", field, name, v)
			}
		}
		if !e.Check(name, FieldUsername, global).IsAllowed() {
			t.Fatalf("global username %s changed", name)
		}
	}
	allowed := []string{
		"alice", "boblee", "badminton", "pingpong", "gongcheng", "beijing",
		"jihui", "xueshu", "caiwu", "jianghu", "luoma", "lizhi", "wangyi",
		"weekend", "photowang", "TTLAlice", "TTLBob",
	}
	for _, name := range allowed {
		if !e.Check(name, FieldUsername, cn).IsAllowed() {
			t.Fatalf("false positive %s", name)
		}
	}
	for _, name := range []string{"Ada", "摄影师小王", "周末爬山群", "共产党", "习近平"} {
		if !e.Check(name, FieldDisplayName, cn).IsAllowed() {
			t.Fatalf("false positive display %s", name)
		}
	}
	if e.Check("共产党专制", FieldDisplayName, cn).IsAllowed() {
		t.Fatal("existing display phrase was dropped")
	}
	if e.Check("gongchandangfubai", FieldUsername, cn).RuleID != "regions/cn/politics.toml:39" {
		t.Fatalf("longer existing term lost: %+v", e.Check("gongchandangfubai", FieldUsername, cn))
	}

	ctx := context.Background()
	const userID int64 = 910173001
	const channelID int64 = 910173002
	t.Setenv(EnvEnabled, "")
	if err := RejectUser(ctx, userID, "xijinping"); err != nil {
		t.Fatalf("flag off: %v", err)
	}
	t.Setenv(EnvEnabled, "1")
	for _, name := range []string{"gongchandang", "xijinping", "g0ngchandang88", "xijinping_01"} {
		if status.Convert(RejectUser(ctx, userID, name)).Message() != "USERNAME_OCCUPIED" {
			t.Fatalf("user %s: %v", name, RejectUser(ctx, userID, name))
		}
		if status.Convert(RejectChannel(ctx, channelID, name)).Message() != "USERNAME_OCCUPIED" {
			t.Fatalf("channel %s: %v", name, RejectChannel(ctx, channelID, name))
		}
		if status.Convert(RejectBot(ctx, userID, name)).Message() != "USERNAME_OCCUPIED" {
			t.Fatalf("bot %s: %v", name, RejectBot(ctx, userID, name))
		}
		if status.Convert(RejectDisplay(ctx, mtproto.PEER_USER, userID, name)).Message() != "FIRSTNAME_INVALID" {
			t.Fatalf("display %s: %v", name, RejectDisplay(ctx, mtproto.PEER_USER, userID, name))
		}
	}
	for _, name := range []string{"alice", "pingpong", "jihui", "TTLAlice", "TTLBob"} {
		if err := RejectUser(ctx, userID, name); err != nil {
			t.Fatalf("allowed user %s: %v", name, err)
		}
		if err := RejectChannel(ctx, channelID, name); err != nil {
			t.Fatalf("allowed channel %s: %v", name, err)
		}
		if err := RejectBot(ctx, userID, name); err != nil {
			t.Fatalf("allowed bot %s: %v", name, err)
		}
	}
}
