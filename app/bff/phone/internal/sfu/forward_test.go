package sfu

import (
	"bytes"
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/pion/rtcp"
	"github.com/pion/rtp"
	"github.com/pion/srtp/v2"
)

func newCtxPair(t *testing.T, prof srtp.ProtectionProfile) (*srtp.Context, *srtp.Context) {
	t.Helper()
	kl, _ := prof.KeyLen()
	sl, _ := prof.SaltLen()
	key := make([]byte, kl)
	salt := make([]byte, sl)
	_, _ = rand.Read(key)
	_, _ = rand.Read(salt)
	a, err := srtp.CreateContext(key, salt, prof)
	if err != nil {
		t.Fatal(err)
	}
	b, err := srtp.CreateContext(key, salt, prof)
	if err != nil {
		t.Fatal(err)
	}
	return a, b
}

type fwdPeer struct {
	conn *net.UDPConn
	enc  *srtp.Context // client side: encrypts what it sends
	dec  *srtp.Context // client side: decrypts what the SFU forwards
}

// setupForward wires two endpoints of one call into a live Server with
// pre-shared SRTP keys (the DTLS part is covered by gcm_handshake_test.go).
func setupForward(t *testing.T, prof srtp.ProtectionProfile) (*Server, [2]*fwdPeer) {
	t.Helper()
	s, err := Listen("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	var peers [2]*fwdPeer
	for i := range peers {
		c, err := net.DialUDP("udp", nil, s.conn.LocalAddr().(*net.UDPAddr))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = c.Close() })
		cliEnc, sfuDec := newCtxPair(t, prof)
		sfuEnc, cliDec := newCtxPair(t, prof)
		ep := &endpoint{callID: 9, key: string(rune('a' + i)), addr: c.LocalAddr().(*net.UDPAddr), dec: sfuDec, enc: sfuEnc, server: s}
		ep.ready.Store(true)
		s.mu.Lock()
		s.byAddr[ep.addr.String()] = ep
		s.calls[9] = append(s.calls[9], ep)
		s.mu.Unlock()
		peers[i] = &fwdPeer{conn: c, enc: cliEnc, dec: cliDec}
	}
	return s, peers
}

func recv(t *testing.T, c *net.UDPConn) []byte {
	t.Helper()
	buf := make([]byte, 2048)
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, err := c.Read(buf)
	if err != nil {
		t.Fatalf("no forwarded packet: %v", err)
	}
	return buf[:n]
}

var forwardProfiles = map[string]srtp.ProtectionProfile{
	"AES128_CM_HMAC_SHA1_80": srtp.ProtectionProfileAes128CmHmacSha1_80,
	"AEAD_AES_128_GCM":       srtp.ProtectionProfileAeadAes128Gcm,
	"AEAD_AES_256_GCM":       srtp.ProtectionProfileAeadAes256Gcm,
}

func TestForwardRTPByteExact(t *testing.T) {
	for name, prof := range forwardProfiles {
		t.Run(name, func(t *testing.T) {
			_, p := setupForward(t, prof)
			for seq := 0; seq < 20; seq++ {
				payload := make([]byte, 40+seq)
				_, _ = rand.Read(payload)
				orig := &rtp.Packet{
					Header: rtp.Header{
						Version: 2, Marker: seq%5 == 0, PayloadType: 111,
						SequenceNumber: uint16(1000 + seq), Timestamp: uint32(48000 + 960*seq), SSRC: 0x1234abcd,
					},
					Payload: payload,
				}
				if seq%2 == 1 { // audio level header extension like tgcalls sends
					if err := orig.Header.SetExtension(1, []byte{0x80 | byte(seq)}); err != nil {
						t.Fatal(err)
					}
				}
				raw, err := orig.Marshal()
				if err != nil {
					t.Fatal(err)
				}
				enc, err := p[0].enc.EncryptRTP(nil, raw, nil)
				if err != nil {
					t.Fatal(err)
				}
				if _, err := p[0].conn.Write(enc); err != nil {
					t.Fatal(err)
				}
				got, err := p[1].dec.DecryptRTP(nil, recv(t, p[1].conn), nil)
				if err != nil {
					t.Fatalf("seq %d: decrypt forwarded: %v", seq, err)
				}
				if !bytes.Equal(got, raw) {
					t.Fatalf("seq %d: forwarded packet differs\n got  %x\n want %x", seq, got, raw)
				}
				var back rtp.Packet
				if err := back.Unmarshal(got); err != nil {
					t.Fatal(err)
				}
				h := back.Header
				if h.SSRC != orig.SSRC || h.SequenceNumber != orig.SequenceNumber || h.Timestamp != orig.Timestamp ||
					h.PayloadType != orig.PayloadType || h.Marker != orig.Marker || h.Extension != orig.Extension ||
					!bytes.Equal(back.Payload, payload) {
					t.Fatalf("seq %d: header/payload mismatch: %+v", seq, h)
				}
			}
		})
	}
}

func TestForwardRTCPNotTreatedAsRTP(t *testing.T) {
	for name, prof := range forwardProfiles {
		t.Run(name, func(t *testing.T) {
			_, p := setupForward(t, prof)
			rr := &rtcp.ReceiverReport{SSRC: 0x55, Reports: []rtcp.ReceptionReport{{SSRC: 0x1234abcd, LastSequenceNumber: 77}}}
			raw, err := rr.Marshal()
			if err != nil {
				t.Fatal(err)
			}
			enc, err := p[0].enc.EncryptRTCP(nil, raw, nil)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = p[0].conn.Write(enc)
			got, err := p[1].dec.DecryptRTCP(nil, recv(t, p[1].conn), nil)
			if err != nil {
				t.Fatalf("decrypt forwarded SRTCP: %v", err)
			}
			if !bytes.Equal(got, raw) {
				t.Fatalf("forwarded RTCP differs\n got  %x\n want %x", got, raw)
			}
		})
	}
}

func TestForwardDropsNonRTP(t *testing.T) {
	_, p := setupForward(t, srtp.ProtectionProfileAeadAes128Gcm)
	for _, b0 := range []byte{64, 79, 100, 0x40 | 0x3f, 200, 255} {
		pkt := make([]byte, 40)
		_, _ = rand.Read(pkt)
		pkt[0] = b0
		_, _ = p[0].conn.Write(pkt)
	}
	buf := make([]byte, 2048)
	_ = p[1].conn.SetReadDeadline(time.Now().Add(300 * time.Millisecond))
	if n, err := p[1].conn.Read(buf); err == nil {
		t.Fatalf("non-RTP datagram was forwarded: %x", buf[:n])
	}
	if isRTCP([]byte{0x80, 111}) || !isRTCP([]byte{0x80, 201}) || !isRTCP([]byte{0x81, 205}) {
		t.Fatal("isRTCP classification wrong")
	}
}
