package sfu

import (
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/pion/stun"
)

func TestSTUNBindingUsesServerPassword(t *testing.T) {
	s, err := Listen("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	answer, ssrc, video, err := s.Join(9, "p", `{"ssrc":55,"ufrag":"cu","pwd":"client-password-value","fingerprints":[{"hash":"sha-256","fingerprint":"AA:BB"}]}`, "9")
	if err != nil || ssrc != 55 || video {
		t.Fatal(answer, ssrc, video, err)
	}
	var parsed struct {
		Transport struct {
			Ufrag string `json:"ufrag"`
			Pwd   string `json:"pwd"`
		} `json:"transport"`
	}
	if err := json.Unmarshal([]byte(answer), &parsed); err != nil {
		t.Fatal(err)
	}
	req, err := stun.Build(
		stun.BindingRequest,
		stun.NewUsername(parsed.Transport.Ufrag+":cu"),
		stun.NewShortTermIntegrity(parsed.Transport.Pwd),
		stun.Fingerprint,
	)
	if err != nil {
		t.Fatal(err)
	}
	c, err := net.DialUDP("udp", nil, s.conn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Write(req.Raw); err != nil {
		t.Fatal(err)
	}
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	buf := make([]byte, 1500)
	n, err := c.Read(buf)
	if err != nil {
		t.Fatal(err)
	}
	res := &stun.Message{Raw: append([]byte(nil), buf[:n]...)}
	if err := res.Decode(); err != nil {
		t.Fatal(err)
	}
	if res.Type != stun.BindingSuccess {
		t.Fatalf("type %s", res.Type)
	}
	var xor stun.XORMappedAddress
	if err := xor.GetFrom(res); err != nil {
		t.Fatal(err)
	}
	if xor.Port == 0 {
		t.Fatal("mapped port")
	}
}
