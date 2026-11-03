package usernamepolicy

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

func TestCorpus(t *testing.T) {
	e := officialEngine()
	allowN, allowFail := checkCorpus(t, e, "corpus/must-allow.txt", true)
	denyN, denyFail := checkCorpus(t, e, "corpus/must-deny.txt", false)
	t.Logf("official policy-2026092301: must-allow %d checked, %d failed; must-deny %d checked, %d failed", allowN, allowFail, denyN, denyFail)
	if allowN != 20029 || denyN != 63 {
		t.Fatalf("corpus size allow=%d deny=%d, want 20029 and 63", allowN, denyN)
	}
	if allowFail > 0 || denyFail > 0 {
		t.Fatalf("corpus failures: allow %d, deny %d", allowFail, denyFail)
	}
}

// mustAllowWeiboLine is the only vendored must-allow row the D-56 overlay
// refuses. The corpus file is not edited. D-56; the user confirmed blocking
// weibo on 2026-11-04.
const mustAllowWeiboLine = 8525

func TestOverlayKeepsCorpus(t *testing.T) {
	e := runtimeEngine()
	allowN, allowFail, weiboDenied := checkAllowExceptWeibo(t, e)
	denyN, denyFail := checkCorpus(t, e, "corpus/must-deny.txt", false)
	if !weiboDenied {
		t.Fatal("overlay must refuse must-allow.txt:8525 username global weibo")
	}
	if allowN != 20028 || allowFail > 0 || denyN != 63 || denyFail > 0 {
		t.Fatalf("overlay corpus: allow %d checked %d failed, deny %d checked %d failed", allowN, allowFail, denyN, denyFail)
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

func checkAllowExceptWeibo(t *testing.T, e *Engine) (checked, failed int, weiboDenied bool) {
	t.Helper()
	f, err := os.Open("corpus/must-allow.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	lineNo := 0
	for sc.Scan() {
		lineNo++
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		field, regions, input, ok := splitCorpus(line)
		if !ok {
			t.Fatalf("corpus/must-allow.txt:%d: want `<field> <regions> <input>`", lineNo)
		}
		v := e.Check(input, field, strings.Split(regions, ","))
		if lineNo == mustAllowWeiboLine {
			if line != "username global weibo" {
				t.Fatalf("line %d is %q, want the weibo exception", lineNo, line)
			}
			weiboDenied = !v.IsAllowed()
			continue
		}
		checked++
		if v.IsAllowed() {
			continue
		}
		failed++
		t.Errorf("corpus/must-allow.txt:%d %q allowed=false outcome=%s rule=%s", lineNo, input, v.Outcome, v.RuleID)
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if !weiboDenied && checked+failed == 0 {
		t.Fatal("did not see the weibo exception line")
	}
	return checked, failed, weiboDenied
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
