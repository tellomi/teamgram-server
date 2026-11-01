package main

import (
	"encoding/csv"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// TestSmokeListMatchesImplementedP0P1 fails when a new implemented P0/P1
// method is missing from the gotd smoke list. Regenerate with
// python3 e2e/client/gen_smoke.py — see docs/testing.md.
func TestSmokeListMatchesImplementedP0P1(t *testing.T) {
	root := filepath.Join("..", "..")
	f, err := os.Open(filepath.Join(root, "docs", "api-coverage.csv"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, err := csv.NewReader(f).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{}
	for i, row := range rows {
		if i == 0 || len(row) < 9 {
			continue
		}
		if row[8] == "implemented" && (row[2] == "P0" || row[2] == "P1") {
			want[row[0]] = true
		}
	}
	raw, err := os.ReadFile(filepath.Join(root, "e2e", "client", "smoke_methods.go"))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, m := range regexp.MustCompile(`Name:\s+"([^"]+)"`).FindAllStringSubmatch(string(raw), -1) {
		got[m[1]] = true
	}
	for name := range want {
		if !got[name] {
			t.Errorf("smoke list missing %s", name)
		}
	}
	for name := range got {
		if !want[name] {
			t.Errorf("smoke list has %s which is not an implemented P0/P1 method", name)
		}
	}
	if len(want) == 0 || len(got) != len(want) {
		t.Fatalf("coverage %d smoke %d", len(want), len(got))
	}
}
