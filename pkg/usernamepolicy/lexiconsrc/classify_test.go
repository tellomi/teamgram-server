package lexiconsrc

import (
	"os"
	"strings"
	"testing"
)

func TestBlobsAndCoverage(t *testing.T) {
	if err := CheckBlobs(); err != nil {
		t.Fatal(err)
	}
	rep, err := Classify()
	if err != nil {
		t.Fatal(err)
	}
	if rep.PinyinOf["成克杰"] != "chengkejie" {
		t.Fatalf("成克杰 pinyin %q", rep.PinyinOf["成克杰"])
	}
	if rep.PinyinOf["共产党腐败"] != "gongchandangfubai" || rep.PinyinOf["抵制共产党"] != "dizhigongchandang" {
		t.Fatalf("compound pinyin %+v", rep.PinyinOf)
	}
	if _, ok := rep.PinyinOf["共产党"]; ok {
		t.Fatal("bare 共产党 was admitted")
	}
	if _, ok := rep.PinyinOf["习近平"]; ok {
		t.Fatal("习近平 was admitted")
	}
	both := strings.Join(rep.DeferBoth, ",")
	if both != "刘瑞龙,周建南,李干成,薄熙,贺子珍,黄敬,黄镇" {
		t.Fatalf("defer-both %s", both)
	}
	cover, err := CoverageJSON()
	if err != nil {
		t.Fatal(err)
	}
	if cover["tencent_file_vendored"] != false {
		t.Fatal("Tencent file must not be vendored")
	}
	if cover["konsheng_commit"] != "d967c30b053fa40b06c5a0dddf0be493f2dfae46" {
		t.Fatalf("konsheng commit %v", cover["konsheng_commit"])
	}
	if cover["fwwdn_commit"] != "a7d06bb1c321e669943b6841570d9da6dad8ce2b" {
		t.Fatalf("fwwdn commit %v", cover["fwwdn_commit"])
	}
	for _, key := range []string{"admit", "pinyin_rules", "policy_rules"} {
		num, _ := cover[key].(float64)
		want := AdmitCount
		if key == "pinyin_rules" {
			want = PinyinRuleCount
		}
		if key == "policy_rules" {
			want = PolicyRuleCount
		}
		if int(num) != want {
			t.Fatalf("%s = %v", key, cover[key])
		}
	}
	src, err := os.ReadFile("SOURCE.toml")
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	for _, needle := range []string{
		"d967c30b053fa40b06c5a0dddf0be493f2dfae46",
		"a7d06bb1c321e669943b6841570d9da6dad8ce2b",
		"2d929be88bdaad438a60b049fa989250c855993a",
		"MIT",
		"Apache-2.0",
		"pypinyin",
		"0.55.0",
		"901d77198aaaadc729e665c10a38321fec8e6592",
		"generated/gfw-selected.tsv",
		"editorial/gfw-stems.txt",
	} {
		if !strings.Contains(text, needle) {
			t.Fatalf("SOURCE.toml missing %s", needle)
		}
	}
}

func TestFwwdnPoliticsIsTheKonshengCopy(t *testing.T) {
	rawK, err := pack.ReadFile("konsheng/政治类型.txt")
	if err != nil {
		t.Fatal(err)
	}
	rawF, err := pack.ReadFile("fwwdn/政治类.txt")
	if err != nil {
		t.Fatal(err)
	}
	k := parseLines(strings.TrimPrefix(string(rawK), "\uFEFF"), false)
	f := parseLines(strings.TrimPrefix(string(rawF), "\uFEFF"), true)
	if len(k) != len(f) {
		t.Fatalf("lines %d vs %d", len(k), len(f))
	}
	for i := range k {
		if k[i] != f[i] {
			t.Fatalf("line %d %q != %q", i, k[i], f[i])
		}
	}
}
