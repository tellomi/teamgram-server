package usernamepolicy

import (
	"os/exec"
	"strings"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/usernamepolicy/lexiconsrc"
)

func TestRuntimePromotionStatus(t *testing.T) {
	terms, err := lexiconsrc.PromotedTerms()
	if err != nil {
		t.Fatal(err)
	}
	pinyin, err := lexiconsrc.RuntimePinyin()
	if err != nil {
		t.Fatal(err)
	}
	if len(terms) != 687 || len(pinyin) != 432 {
		t.Fatalf("promoted %d pinyin %d", len(terms), len(pinyin))
	}
	byTerm := map[string]lexiconsrc.RuntimeTerm{}
	for _, term := range terms {
		byTerm[term.Term] = term
		for _, path := range term.Files {
			if !strings.HasPrefix(path, "konsheng/") {
				t.Fatalf("%s attributed to %s", term.Term, path)
			}
		}
	}
	for _, term := range []string{"共产党", "习近平"} {
		got, ok := byTerm[term]
		if !ok || !containsStr(got.Files, "konsheng/政治类型.txt") {
			t.Fatalf("%s files %+v", term, got.Files)
		}
	}
	gcd, ok := byTerm["gongchandang"]
	if !ok || !containsStr(gcd.Files, "konsheng/贪腐词库.txt") {
		t.Fatalf("gongchandang %+v", gcd)
	}
	if _, ok := byTerm["xijinping"]; ok {
		t.Fatal("xijinping is not a line of the three category files")
	}
	if pinyin["习近平"] != "xijinping" {
		t.Fatalf("习近平 pinyin %q", pinyin["习近平"])
	}
	for _, term := range []string{
		"政府", "西藏", "拉萨", "解放军", "胡总", "李鹏", "台湾", "taiwan", "gfw", "youxing",
		"炸弹配方", "刘瑞龙", "贺子珍", "贪污", "双开",
	} {
		if _, ok := byTerm[term]; ok {
			t.Fatalf("deferred %s was promoted", term)
		}
	}

	e := runtimeEngine()
	if e.RuleCount() != 1024+687+432 {
		t.Fatalf("runtime rules %d", e.RuleCount())
	}
	cn := []string{"global", "cn"}
	global := []string{"global"}
	for _, name := range []string{
		"gongchandang", "GONGCHANDANG", "g0ngchandang", "gongchandang88", "my_gongchandang",
		"xijinping", "XiJinPing", "x1jinping", "xijinping_01", "01_xijinping",
		"hujintao", "HUJINTAO", "wenjiabao",
	} {
		if e.Check(name, FieldUsername, cn).IsAllowed() {
			t.Fatalf("username %s was allowed", name)
		}
		if !e.Check(name, FieldUsername, global).IsAllowed() {
			t.Fatalf("global %s changed", name)
		}
	}
	if !e.Check("notgongchandangx", FieldUsername, cn).IsAllowed() {
		t.Fatal("buried latin was blocked")
	}
	if !e.Check("notxijinpingx", FieldUsername, cn).IsAllowed() {
		t.Fatal("buried xijinping was blocked")
	}
	for _, name := range []string{"习近平", "习近1平", "共产党", "共产1党", "胡锦涛", "法轮功"} {
		if e.Check(name, FieldDisplayName, cn).IsAllowed() {
			t.Fatalf("display %s was allowed", name)
		}
		if !e.Check(name, FieldDisplayName, global).IsAllowed() {
			t.Fatalf("display %s applied outside cn", name)
		}
	}
	for _, name := range []string{"alice", "lvdebin1", "xjp1", "taidu1", "政府", "李鹏", "摄影师小王"} {
		field := FieldUsername
		if strings.ContainsAny(name, "政李摄") {
			field = FieldDisplayName
		}
		if !e.Check(name, field, cn).IsAllowed() {
			t.Fatalf("false positive %s", name)
		}
	}
	// The published longer phrase still wins over the new shorter term.
	v := e.Check("共产党专制", FieldDisplayName, cn)
	if v.IsAllowed() || !strings.Contains(v.RuleID, "regions/cn/politics.toml") {
		t.Fatalf("longer published phrase %+v", v)
	}
}

func TestRuntimePinyinRecheck(t *testing.T) {
	cmd := exec.Command("python3", "-c", "import pypinyin")
	if err := cmd.Run(); err != nil {
		t.Skip("pypinyin is not installed")
	}
	out, err := exec.Command("python3", "lexiconsrc/update.py", "--check-runtime").CombinedOutput()
	if err != nil {
		t.Fatalf("check-runtime: %v\n%s", err, out)
	}
}
