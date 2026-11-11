package gnet

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestProxyRejectsPartialHeader(t *testing.T) {
	gw := startProxyGateway(t, "v1v2", "127.0.0.1/32", true)
	conn := dialProxy(t, gw.proxy)
	defer conn.Close()
	if _, err := conn.Write([]byte("PROXY TCP4 203.0.113.8 10.0.0.1 1234")); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(700 * time.Millisecond))
	buf := make([]byte, 8)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("partial header was accepted")
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("partial header closed immediately: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("partial header stayed open")
	}
	if ips := gw.pipe.ClientIPs(); len(ips) != 0 {
		t.Fatalf("partial header wrote client_ip %v", ips)
	}
}

func TestProxyRejectsOversizedHeader(t *testing.T) {
	gw := startProxyGateway(t, "v1v2", "127.0.0.1/32", true)
	conn := dialProxy(t, gw.proxy)
	defer conn.Close()
	buf := make([]byte, 16)
	copy(buf, []byte{0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 0x51, 0x55, 0x49, 0x54, 0x0a})
	buf[12] = 0x21
	buf[13] = 0x11
	binary.BigEndian.PutUint16(buf[14:16], 2049)
	expectClosedSoon(t, conn, buf)
	if ips := gw.pipe.ClientIPs(); len(ips) != 0 {
		t.Fatalf("oversized header wrote client_ip %v", ips)
	}
}

func TestProxyRejectsDuplicateHeader(t *testing.T) {
	gw := startProxyGateway(t, "v1v2", "127.0.0.1/32", true)
	conn := dialProxy(t, gw.proxy)
	defer conn.Close()
	first := proxyV2TCP4("203.0.113.10", 11)
	second := proxyV2TCP4("203.0.113.11", 12)
	expectClosedSoon(t, conn, append(append([]byte{}, first...), second...))
	for _, ip := range gw.pipe.ClientIPs() {
		if ip == "203.0.113.10" || ip == "203.0.113.11" {
			t.Fatalf("duplicate header stored %s", ip)
		}
	}
}

func TestProxyRejectsLocalCommand(t *testing.T) {
	gw := startProxyGateway(t, "v1v2", "127.0.0.1/32", true)
	conn := dialProxy(t, gw.proxy)
	defer conn.Close()
	buf := make([]byte, 16)
	copy(buf, []byte{0x0d, 0x0a, 0x0d, 0x0a, 0x00, 0x0d, 0x0a, 0x51, 0x55, 0x49, 0x54, 0x0a})
	buf[12] = 0x20
	expectClosedSoon(t, conn, buf)
}

func TestProxyRejectsUDP(t *testing.T) {
	gw := startProxyGateway(t, "v1v2", "127.0.0.1/32", true)
	conn := dialProxy(t, gw.proxy)
	defer conn.Close()
	buf := proxyV2(0x12, net.IPv4(198, 51, 100, 9).To4(), net.IPv4(10, 0, 0, 1).To4(), 9, 443)
	expectClosedSoon(t, conn, buf)
	for _, ip := range gw.pipe.ClientIPs() {
		if ip == "198.51.100.9" {
			t.Fatal("udp header became client_ip")
		}
	}
}

func TestProxyRejectsUntrustedPeer(t *testing.T) {
	gw := startProxyGateway(t, "v1v2", "172.19.44.222/32", true)
	conn := dialProxy(t, gw.proxy)
	defer conn.Close()
	expectClosedSoon(t, conn, proxyV2TCP4("203.0.113.77", 77))
	for _, ip := range gw.pipe.ClientIPs() {
		if ip == "203.0.113.77" || ip == "172.19.44.222" {
			t.Fatalf("untrusted peer stored %s", ip)
		}
	}
}

func TestMappedIPv4MatchesSingleHost(t *testing.T) {
	spec := proxyListenerSpec{peers: mustCIDR(t, "172.19.44.222/32")}
	mapped := net.ParseIP("::ffff:172.19.44.222")
	if !spec.allows(mapped) {
		t.Fatal("::ffff:172.19.44.222 did not match 172.19.44.222/32")
	}
	if spec.allows(net.ParseIP("172.19.44.223")) {
		t.Fatal("neighbor matched /32")
	}
}

func mustCIDR(t *testing.T, cidr string) []*net.IPNet {
	t.Helper()
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		t.Fatal(err)
	}
	return []*net.IPNet{network}
}

func dialProxy(t *testing.T, addr string) net.Conn {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	return conn
}

func expectClosedSoon(t *testing.T, conn net.Conn, payload []byte) {
	t.Helper()
	_ = conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write(payload); err != nil && err != io.EOF {
		// A reset while writing is the server closing.
		return
	}
	buf := make([]byte, 8)
	if _, err := conn.Read(buf); err == nil {
		t.Fatal("connection stayed open")
	}
}
