// Package relay is the 1:1 media edge: a Telegram peer_tag UDP reflector
// and coturn REST credentials for phoneConnectionWebrtc.
package relay

import (
	"encoding/hex"
	"net"
	"sync"
)

// Reflector is the classic Telegram phone relay. Each side sends UDP
// datagrams prefixed with its own 16-byte peer_tag. The reflector learns
// the sender's address from that tag and forwards the payload to the other
// side prefixed with the other side's tag, which is what libtgvoip expects.
type Reflector struct {
	conn *net.UDPConn

	mu    sync.Mutex
	peers map[string]*leg
	done  chan struct{}
}

type leg struct {
	other *leg
	addr  *net.UDPAddr
	tag   []byte
}

// Listen binds the reflector. port 0 asks the kernel for a free port.
func Listen(port int) (*Reflector, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		return nil, err
	}
	r := &Reflector{
		conn:  conn,
		peers: map[string]*leg{},
		done:  make(chan struct{}),
	}
	go r.loop()
	return r, nil
}

func (r *Reflector) Addr() *net.UDPAddr {
	if r == nil || r.conn == nil {
		return nil
	}
	return r.conn.LocalAddr().(*net.UDPAddr)
}

func (r *Reflector) Close() error {
	if r == nil {
		return nil
	}
	select {
	case <-r.done:
	default:
		close(r.done)
	}
	return r.conn.Close()
}

// Pair registers the two peer tags of one call. Tags are 16 bytes.
func (r *Reflector) Pair(tagA, tagB []byte) {
	if r == nil || len(tagA) != 16 || len(tagB) != 16 {
		return
	}
	a := &leg{tag: append([]byte(nil), tagA...)}
	b := &leg{tag: append([]byte(nil), tagB...)}
	a.other = b
	b.other = a
	r.mu.Lock()
	r.peers[hex.EncodeToString(tagA)] = a
	r.peers[hex.EncodeToString(tagB)] = b
	r.mu.Unlock()
}

// Unpair drops both tags.
func (r *Reflector) Unpair(tagA, tagB []byte) {
	if r == nil {
		return
	}
	r.mu.Lock()
	delete(r.peers, hex.EncodeToString(tagA))
	delete(r.peers, hex.EncodeToString(tagB))
	r.mu.Unlock()
}

func (r *Reflector) loop() {
	buf := make([]byte, 2048)
	for {
		n, addr, err := r.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if n < 16 {
			continue
		}
		tag := hex.EncodeToString(buf[:16])
		r.mu.Lock()
		self := r.peers[tag]
		if self != nil {
			self.addr = addr
		}
		var other *leg
		if self != nil {
			other = self.other
		}
		r.mu.Unlock()
		if other == nil || other.addr == nil || n == 16 {
			continue
		}
		out := make([]byte, 16+(n-16))
		copy(out, other.tag)
		copy(out[16:], buf[16:n])
		_, _ = r.conn.WriteToUDP(out, other.addr)
	}
}
