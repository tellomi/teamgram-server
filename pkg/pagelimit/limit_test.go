package pagelimit

import "testing"

func TestCap(t *testing.T) {
	if got := Cap(0); got != 0 {
		t.Fatalf("zero %d", got)
	}
	if got := Cap(-1); got != -1 {
		t.Fatalf("negative %d", got)
	}
	if got := Cap(55); got != 55 {
		t.Fatalf("55 %d", got)
	}
	if got := Cap(100); got != 100 {
		t.Fatalf("100 %d", got)
	}
	if got := Cap(101); got != Max {
		t.Fatalf("101 %d", got)
	}
}

func TestOrDefault(t *testing.T) {
	if got := OrDefault(0, 20); got != 20 {
		t.Fatalf("missing %d", got)
	}
	if got := OrDefault(100, 20); got != 100 {
		t.Fatalf("100 %d", got)
	}
	if got := OrDefault(1000, 20); got != Max {
		t.Fatalf("1000 %d", got)
	}
}
