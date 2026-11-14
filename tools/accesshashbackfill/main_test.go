package main

import "testing"

func TestNextHashIsNeverZero(t *testing.T) {
	for i := 0; i < 32; i++ {
		h, err := nextHash()
		if err != nil {
			t.Fatal(err)
		}
		if h == 0 {
			t.Fatal("zero hash")
		}
	}
}
