package usernamepolicy

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
)

func TestCnUsernameSupplement(t *testing.T) {
	if CnUsernameSupplementStats() < 188 {
		t.Fatalf("supplement shrunk below the published 188: %d", CnUsernameSupplementStats())
	}
	e := runtimeEngine()
	published := map[string]struct{}{}
	for _, rule := range currentEngine().rules {
		if !strings.Contains(rule.Source, "regions/cn/politics.toml") || !containsStr(rule.Fields, FieldUsername) {
			continue
		}
		sk := letterSkeleton(rule.Normalized)
		if len([]rune(sk)) < cnUsernameSupplementMinRunes {
			continue
		}
		published[sk] = struct{}{}
	}
	if len(published) != 188 {
		t.Fatalf("published long latin %d", len(published))
	}
	for _, term := range e.cnSupplementTerms() {
		delete(published, term.sk)
	}
	if len(published) != 0 {
		t.Fatalf("published skeletons missing from the supplement: %d", len(published))
	}
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
			if v.IsAllowed() || v.Outcome != OutcomeRegionalRestricted {
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
	for _, name := range []string{"Ada", "摄影师小王", "周末爬山群", "政府", "西藏", "胡总", "贪污", "双开", "刘瑞龙"} {
		if !e.Check(name, FieldDisplayName, cn).IsAllowed() {
			t.Fatalf("false positive display %s", name)
		}
	}
	for _, name := range []string{"习近平", "习近1平", "反对习近平", "共产党", "共产1党"} {
		v := e.Check(name, FieldDisplayName, cn)
		if v.IsAllowed() {
			t.Fatalf("han %s was allowed", name)
		}
	}
	if e.Check("习近1平", FieldDisplayName, cn).Matched != "CONTAINS_DIGIT" {
		t.Fatalf("digit han %+v", e.Check("习近1平", FieldDisplayName, cn))
	}
	for _, name := range []string{"gongchandang88", "xijinping88", "88gongchandang"} {
		v := e.Check(name, FieldUsername, cn)
		if v.IsAllowed() || !strings.HasPrefix(v.RuleID, "supplement/cn-username:") {
			t.Fatalf("digit boundary %s %+v", name, v)
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
