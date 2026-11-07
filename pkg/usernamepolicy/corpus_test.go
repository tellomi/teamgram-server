package usernamepolicy

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestCurrentCorpus(t *testing.T) {
	e := runtimeEngine()
	if e.Version() != 2026110602 {
		t.Fatalf("version %d", e.Version())
	}
	if e.RuleCount() != 1024 {
		t.Fatalf("rules %d", e.RuleCount())
	}
	allowN, allowFail := checkCorpus(t, e, "corpus/must-allow.txt", true)
	denyN, denyFail := checkCorpus(t, e, "corpus/must-deny.txt", false)
	t.Logf("policy-2026110602: must-allow %d checked, %d failed; must-deny %d checked, %d failed", allowN, allowFail, denyN, denyFail)
	if allowN != 20157 || denyN != 218 {
		t.Fatalf("corpus size allow=%d deny=%d, want 20157 and 218", allowN, denyN)
	}
	if allowFail > 0 || denyFail > 0 {
		t.Fatalf("corpus failures: allow %d, deny %d", allowFail, denyFail)
	}
}

func TestWeiboD24(t *testing.T) {
	e := runtimeEngine()
	global := []string{"global"}
	for _, name := range []string{"weibo", "weibo_01", "my_weibo"} {
		if e.Check(name, FieldUsername, global).IsAllowed() {
			t.Fatalf("%s should be refused", name)
		}
	}
	if !e.Check("weibo88", FieldUsername, global).IsAllowed() {
		t.Fatal("weibo88 should be allowed (D-24)")
	}
}

func TestD24OnOfficialPolicy(t *testing.T) {
	e := officialEngine()
	global := []string{"global"}
	for _, name := range []string{"admin01", "support88"} {
		if !e.Check(name, FieldUsername, global).IsAllowed() {
			t.Fatalf("%s should be allowed (D-24)", name)
		}
	}
	for _, name := range []string{"admin", "admin_01", "support_88", "tellomi_xxx"} {
		if e.Check(name, FieldUsername, global).IsAllowed() {
			t.Fatalf("%s should be refused (D-24)", name)
		}
	}
}

func checkCorpus(t *testing.T, e *Engine, path string, expectAllowed bool) (int, int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	checked, failed := 0, 0
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		field, regions, input, ok := splitCorpus(line)
		if !ok {
			t.Fatalf("%s:%d: want `<field> <regions> <input>`", path, lineNo)
		}
		v := e.Check(input, field, strings.Split(regions, ","))
		checked++
		if v.IsAllowed() != expectAllowed {
			failed++
			t.Errorf("%s:%d %q field %s allowed=%v outcome=%s rule=%s", path, lineNo, input, field, v.IsAllowed(), v.Outcome, v.RuleID)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return checked, failed
}

func splitCorpus(line string) (field, regions, input string, ok bool) {
	parts := strings.SplitN(line, " ", 3)
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}
