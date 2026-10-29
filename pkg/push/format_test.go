package push

import "testing"

func TestFormatUIDsRoundTrip(t *testing.T) {
	s := formatUIDs([]int64{1, 2, 99})
	got := parseUIDs(s)
	if len(got) != 3 || got[2] != 99 {
		t.Fatal(got)
	}
	if parseUIDs("") != nil {
		t.Fatal("empty")
	}
	if joinName("Ada", "Lovelace") != "Ada Lovelace" || joinName("Ada", "") != "Ada" {
		t.Fatal(joinName("Ada", "Lovelace"))
	}
}
