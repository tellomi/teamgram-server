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
	supplement, err := lexiconsrc.SupplementTerms()
	if err != nil {
		t.Fatal(err)
	}
	// 687 overlap lines minus 22 editorial deferrals. GFW selection is the
	// stem/marker generator output, not a hand-picked list.
	if len(terms) != 665 || len(supplement) != 379 || len(pinyin) != 721 {
		t.Fatalf("promoted %d allow %d pinyin %d", len(terms), len(supplement), len(pinyin))
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
	if e.RuleCount() != 1024+665+379+721 {
		t.Fatalf("runtime rules %d", e.RuleCount())
	}
	pure, mixed := 0, 0
	for _, term := range append(append([]lexiconsrc.RuntimeTerm{}, terms...), supplement...) {
		han := true
		for _, r := range term.Term {
			if r < 0x3400 || r > 0x9FFF {
				han = false
				break
			}
		}
		if han {
			pure++
		} else {
			mixed++
		}
	}
	if pure != 860 || mixed != 184 {
		t.Fatalf("pure %d mixed %d", pure, mixed)
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

func TestRuntimeEditorialDecisions(t *testing.T) {
	exact, err := lexiconsrc.RuntimeExact()
	if err != nil {
		t.Fatal(err)
	}
	if len(exact) != 4 || exact["八九年"] == "" || exact["贰拾年"] == "" || exact["反分裂"] == "" || exact["打砸抢"] == "" {
		t.Fatalf("exact-only %+v", exact)
	}
	for _, word := range []string{"四事件", "4事件", "上海帮", "北京帮", "上中央", "去中央", "自由门", "军事社", "威胁论"} {
		if !lexiconsrc.FragmentExact(word) {
			t.Fatalf("fragment %s", word)
		}
	}
	for _, word := range []string{"习近平", "共产党", "辛灏年", "零八宪章", "胡耀邦", "八九年"} {
		if lexiconsrc.FragmentExact(word) {
			t.Fatalf("not a fragment %s", word)
		}
	}
	terms, err := lexiconsrc.PromotedTerms()
	if err != nil {
		t.Fatal(err)
	}
	supplement, err := lexiconsrc.SupplementTerms()
	if err != nil {
		t.Fatal(err)
	}
	byTerm := map[string]lexiconsrc.RuntimeTerm{}
	for _, term := range terms {
		byTerm[term.Term] = term
	}
	for _, term := range supplement {
		if _, ok := byTerm[term.Term]; ok {
			t.Fatalf("allowlist duplicates %s", term.Term)
		}
		if len(term.Files) != 1 || term.Files[0] != "generated/gfw-selected.tsv" {
			t.Fatalf("%s files %+v", term.Term, term.Files)
		}
		byTerm[term.Term] = term
	}
	for _, word := range []string{
		"塔利班", "木仓", "火乍", "核蛋", "弓单", "石肖",
		"抵制", "静坐", "平反", "罢课", "霸工",
		"天安门", "新疆", "法轮", "仿真枪", "黄色图片",
	} {
		if _, ok := byTerm[word]; ok {
			t.Fatalf("%s was loaded", word)
		}
	}
	for _, word := range []string{"低制", "底制", "平近习", "静zuo", "罢ke", "八九年", "四事件"} {
		if _, ok := byTerm[word]; !ok {
			t.Fatalf("kept %s", word)
		}
	}
	for _, word := range []string{
		"零八宪章", "八九学运", "天安门事件", "天安门广场大屠杀",
		"胡耀邦", "赵紫阳", "法轮大法", "平反六四", "新疆独立", "六四天安门", "达赖喇嘛",
	} {
		if _, ok := byTerm[word]; !ok {
			t.Fatalf("allow %s missing", word)
		}
	}

	e := runtimeEngine()
	cn := []string{"global", "cn"}
	global := []string{"global"}
	for _, name := range []string{
		"塔利班", "反对塔利班", "木仓", "火乍", "核蛋", "弓单", "石肖",
		"抵制", "静坐", "平反", "罢课", "霸餐",
		"一九八九年", "八九年出生", "八九1年", "纪念贰拾年",
		"天安门", "天安门广场", "新疆", "习近苹", "南京大屠杀", "独立思考",
		"五四事件", "五四运动", "五四青年节", "六四事件", "上海帮忙", "北京帮忙", "清华帮忙",
		"上中央电视台", "去中央大街", "自由门窗", "军事社团", "反分裂国家法", "威胁论调", "打砸抢烧",
	} {
		if v := e.Check(name, FieldDisplayName, cn); !v.IsAllowed() {
			t.Fatalf("false positive %s -> %s %s", name, v.Term, v.RuleID)
		}
	}
	for _, name := range []string{
		"八九年", "低制", "底制", "平近习", "静zuo", "罢ke",
		"零八宪章", "八九学运", "八九学生", "天安门事件", "天安门广场大屠杀",
		"胡耀邦", "赵紫阳", "法轮大法", "转法轮", "平反六四", "64平反",
		"新疆独立", "六四", "六四天安门", "64事件", "达赖喇嘛", "08宪章", "6.4学潮",
		"四事件", "4事件", "上海帮", "上中央", "自由门", "军事社", "反分裂", "威胁论", "打砸抢", "贰拾年", "辛灏年", "关于辛灏年",
	} {
		if e.Check(name, FieldDisplayName, cn).IsAllowed() {
			t.Fatalf("display %s was allowed", name)
		}
		if !e.Check(name, FieldDisplayName, global).IsAllowed() {
			t.Fatalf("display %s applied outside cn", name)
		}
	}
	v := e.Check("抵制共产党", FieldDisplayName, cn)
	if v.IsAllowed() || !strings.Contains(v.RuleID, "regions/cn/politics.toml") {
		t.Fatalf("longer phrase %+v", v)
	}
	var year, charter rule
	for _, item := range e.rules {
		switch item.Term {
		case "八九年":
			year = item
		case "零八宪章":
			charter = item
		}
	}
	if year.Term == "" || strings.Join(year.Matches, ",") != MatchNormalizedExact {
		t.Fatalf("八九年 matches %+v", year)
	}
	var event rule
	for _, item := range e.rules {
		if item.Term == "四事件" {
			event = item
		}
	}
	if event.Term == "" || strings.Join(event.Matches, ",") != MatchNormalizedExact {
		t.Fatalf("四事件 matches %+v", event)
	}
	if !containsStr(charter.Matches, MatchContains) || !strings.Contains(charter.Source, "generated/gfw-selected.tsv") {
		t.Fatalf("零八宪章 %+v", charter)
	}
	for _, name := range []string{"bajiunian", "lingbaxianzhang", "LINGBAXIANZHANG", "huyaobang", "falundafa", "pingfan64", "dalailama", "lihongzhi", "falungong", "gongcandang", "gongchanzhuanzhi"} {
		if e.Check(name, FieldUsername, cn).IsAllowed() {
			t.Fatalf("username %s was allowed", name)
		}
		if !e.Check(name, FieldUsername, global).IsAllowed() {
			t.Fatalf("global %s changed", name)
		}
	}
	for _, name := range []string{"notlingbaxianzhangx", "jingzuo", "xjp1", "xxlingbaxianzhangyy"} {
		if !e.Check(name, FieldUsername, cn).IsAllowed() {
			t.Fatalf("bypass %s was blocked", name)
		}
	}
	for _, name := range []string{"aa_lingbaxianzhang", "lingbaxianzhang_01"} {
		if e.Check(name, FieldUsername, cn).IsAllowed() {
			t.Fatalf("boundary %s was allowed", name)
		}
	}
	if e.Check("o八宪章", FieldDisplayName, cn).IsAllowed() || e.Check("O八宪章", FieldDisplayName, cn).IsAllowed() {
		t.Fatal("O八宪章 case was allowed")
	}
	if e.Check("五四1事件", FieldDisplayName, cn).IsAllowed() == false {
		t.Fatal("五四1事件 was blocked")
	}
	if e.Check("天安1门事件", FieldDisplayName, cn).IsAllowed() {
		t.Fatal("digit form of 天安门事件 was allowed")
	}
	if e.Check("胡耀1邦", FieldDisplayName, cn).IsAllowed() {
		t.Fatal("digit form of 胡耀邦 was allowed")
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
