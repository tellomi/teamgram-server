package http

import "testing"

func TestParseDfsFileName(t *testing.T) {
	creator, id, err := ParseDfsFileName("7_42.mp4")
	if err != nil || creator != 7 || id != 42 {
		t.Fatalf("got %d %d %v", creator, id, err)
	}
	if _, _, err = ParseDfsFileName("x.mp4"); err == nil {
		t.Fatal("x.mp4 was accepted as an object key")
	}
	if _, _, err = ParseDfsFileName("12.mp4"); err == nil {
		t.Fatal("bare id was accepted")
	}
}
