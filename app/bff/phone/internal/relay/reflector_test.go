package relay

import (
	"net"
	"testing"
	"time"
)

func TestReflectorSwapsPeerTag(t *testing.T) {
	r, err := Listen(0)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	tagA := []byte("aaaaaaaaaaaaaaaa")
	tagB := []byte("bbbbbbbbbbbbbbbb")
	r.Pair(tagA, tagB)

	a, err := net.DialUDP("udp", nil, r.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := net.DialUDP("udp", nil, r.Addr())
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	payload := []byte("hello-call")
	if _, err := a.Write(append(append([]byte{}, tagA...), payload...)); err != nil {
		t.Fatal(err)
	}
	// B must register its address before A’s payload can be forwarded.
	if _, err := b.Write(append([]byte{}, tagB...)); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Write(append(append([]byte{}, tagA...), payload...)); err != nil {
		t.Fatal(err)
	}
	_ = b.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 64)
	n, err := b.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	got := buf[:n]
	if n < 16 || string(got[:16]) != string(tagB) || string(got[16:]) != string(payload) {
		t.Fatalf("got %q", got)
	}
}
