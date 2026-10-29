package sfu

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/dtls/v2"
	"github.com/pion/dtls/v2/pkg/crypto/selfsign"
	"github.com/pion/rtp"
	"github.com/pion/srtp/v2"
	"github.com/pion/stun"
)

// Server is an ICE-lite UDP socket that completes DTLS as the client and
// forwards SRTP between participants of one group call. Audio is forwarded
// with the sender's SSRC. Video RTP is forwarded the same way; simulcast
// layer selection (tgcalls colibri) is not implemented.
type Server struct {
	conn *net.UDPConn
	ip   string
	cert tls.Certificate
	fp   string

	mu     sync.Mutex
	byFrag map[string]*endpoint
	byAddr map[string]*endpoint
	calls  map[int64][]*endpoint
	done   chan struct{}
}

type endpoint struct {
	callID   int64
	key      string
	localU   string
	localP   string
	remoteU  string
	remoteP  string
	remoteFP string
	addr     *net.UDPAddr
	dtlsIn   chan []byte
	stop     chan struct{}
	started  bool
	dtls     *dtls.Conn
	dec      *srtp.Context
	enc      *srtp.Context
	ready    atomic.Bool
	record   atomic.Bool
	server   *Server
}

// Listen binds the SFU. ip is the address advertised to clients.
func Listen(ip string, port int) (*Server, error) {
	if ip == "" {
		ip = "127.0.0.1"
	}
	cert, err := selfsign.GenerateSelfSigned()
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: port})
	if err != nil {
		return nil, err
	}
	s := &Server{
		conn:   conn,
		ip:     ip,
		cert:   cert,
		fp:     certFingerprint(cert),
		byFrag: map[string]*endpoint{},
		byAddr: map[string]*endpoint{},
		calls:  map[int64][]*endpoint{},
		done:   make(chan struct{}),
	}
	go s.loop()
	return s, nil
}

func (s *Server) Port() int {
	if s == nil || s.conn == nil {
		return 0
	}
	return s.conn.LocalAddr().(*net.UDPAddr).Port
}

func (s *Server) Close() error {
	if s == nil {
		return nil
	}
	select {
	case <-s.done:
	default:
		close(s.done)
	}
	return s.conn.Close()
}

// Join registers a participant and returns the tgcalls answer JSON plus the
// audio SSRC that should be stored as GroupCallParticipant.source.
func (s *Server) Join(callID int64, key, offerJSON, endpointID string) (answer string, audioSSRC uint32, video bool, err error) {
	if s == nil {
		return "", 0, false, fmt.Errorf("sfu not running")
	}
	offer, err := ParseOffer(offerJSON)
	if err != nil {
		return "", 0, false, err
	}
	local := Local{
		Ufrag:       iceString(8),
		Pwd:         iceString(24),
		Fingerprint: s.fp,
		IP:          s.ip,
		Port:        s.Port(),
		Endpoint:    endpointID,
	}
	answer, err = AnswerJSON(offer, local)
	if err != nil {
		return "", 0, false, err
	}
	ep := &endpoint{
		callID:   callID,
		key:      key,
		localU:   local.Ufrag,
		localP:   local.Pwd,
		remoteU:  offer.Ufrag,
		remoteP:  offer.Pwd,
		remoteFP: offer.Fingerprint,
		dtlsIn:   make(chan []byte, 64),
		stop:     make(chan struct{}),
		server:   s,
	}
	s.mu.Lock()
	s.dropLocked(callID, key)
	s.byFrag[local.Ufrag] = ep
	s.calls[callID] = append(s.calls[callID], ep)
	s.mu.Unlock()
	return answer, offer.AudioSSRC, len(offer.VideoGroups) > 0, nil
}

// Leave removes one participant.
func (s *Server) Leave(callID int64, key string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.dropLocked(callID, key)
	s.mu.Unlock()
}

// SetRecording marks every participant of the call. Decrypted audio payloads
// are counted while it is on; the TL record flags are the client-visible state.
func (s *Server) SetRecording(callID int64, on bool) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, ep := range s.calls[callID] {
		ep.record.Store(on)
	}
}

func (s *Server) dropLocked(callID int64, key string) {
	list := s.calls[callID]
	kept := list[:0]
	for _, ep := range list {
		if ep.key == key {
			delete(s.byFrag, ep.localU)
			if ep.addr != nil {
				delete(s.byAddr, ep.addr.String())
			}
			select {
			case <-ep.stop:
			default:
				close(ep.stop)
			}
			if ep.dtls != nil {
				_ = ep.dtls.Close()
			}
			continue
		}
		kept = append(kept, ep)
	}
	if len(kept) == 0 {
		delete(s.calls, callID)
	} else {
		s.calls[callID] = kept
	}
}

func (s *Server) loop() {
	buf := make([]byte, 2048)
	for {
		n, addr, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if n < 2 {
			continue
		}
		pkt := append([]byte(nil), buf[:n]...)
		b0 := pkt[0]
		switch {
		case b0 < 2:
			s.onSTUN(pkt, addr)
		case b0 >= 20 && b0 <= 63:
			s.onDTLS(pkt, addr)
		default:
			s.onMedia(pkt, addr)
		}
	}
}

func (s *Server) onSTUN(pkt []byte, addr *net.UDPAddr) {
	msg := &stun.Message{Raw: pkt}
	if err := msg.Decode(); err != nil {
		return
	}
	if msg.Type != stun.BindingRequest {
		return
	}
	var user stun.Username
	if err := user.GetFrom(msg); err != nil {
		return
	}
	parts := strings.SplitN(string(user), ":", 2)
	if len(parts) != 2 {
		return
	}
	s.mu.Lock()
	ep := s.byFrag[parts[0]]
	s.mu.Unlock()
	if ep == nil {
		return
	}
	if err := stun.NewShortTermIntegrity(ep.localP).Check(msg); err != nil {
		return
	}
	s.bind(ep, addr)
	res, err := stun.Build(
		msg,
		stun.BindingSuccess,
		&stun.XORMappedAddress{IP: addr.IP, Port: addr.Port},
		stun.NewShortTermIntegrity(ep.localP),
		stun.Fingerprint,
	)
	if err != nil {
		return
	}
	_, _ = s.conn.WriteToUDP(res.Raw, addr)
}

func (s *Server) bind(ep *endpoint, addr *net.UDPAddr) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ep.addr == nil {
		ep.addr = addr
		s.byAddr[addr.String()] = ep
	}
	if !ep.started {
		ep.started = true
		go ep.handshake()
	}
}

func (s *Server) onDTLS(pkt []byte, addr *net.UDPAddr) {
	s.mu.Lock()
	ep := s.byAddr[addr.String()]
	s.mu.Unlock()
	if ep == nil {
		return
	}
	select {
	case ep.dtlsIn <- pkt:
	default:
	}
}

func (s *Server) onMedia(pkt []byte, addr *net.UDPAddr) {
	s.mu.Lock()
	ep := s.byAddr[addr.String()]
	others := append([]*endpoint(nil), s.calls[epCall(ep)]...)
	s.mu.Unlock()
	if ep == nil || !ep.ready.Load() || ep.dec == nil {
		return
	}
	var packet rtp.Packet
	if err := packet.Unmarshal(pkt); err != nil {
		return
	}
	plain, err := ep.dec.DecryptRTP(nil, pkt, &packet.Header)
	if err != nil {
		return
	}
	if ep.record.Load() {
		recordedPackets.Add(1)
	}
	packet.Payload = plain
	raw, err := packet.Marshal()
	if err != nil {
		return
	}
	for _, other := range others {
		if other == nil || other == ep || !other.ready.Load() || other.enc == nil || other.addr == nil {
			continue
		}
		out, err := other.enc.EncryptRTP(nil, raw, &packet.Header)
		if err != nil {
			continue
		}
		_, _ = s.conn.WriteToUDP(out, other.addr)
	}
}

func epCall(ep *endpoint) int64 {
	if ep == nil {
		return 0
	}
	return ep.callID
}

// recordedPackets counts decrypted RTP packets while recording is on.
var recordedPackets atomic.Int64

func RecordedPackets() int64 { return recordedPackets.Load() }

func (ep *endpoint) handshake() {
	vc := &dgramConn{
		in:     ep.dtlsIn,
		stop:   ep.stop,
		remote: ep.addr,
		write: func(p []byte) error {
			_, err := ep.server.conn.WriteToUDP(p, ep.addr)
			return err
		},
	}
	cfg := &dtls.Config{
		Certificates:       []tls.Certificate{ep.server.cert},
		InsecureSkipVerify: true,
		SRTPProtectionProfiles: []dtls.SRTPProtectionProfile{
			dtls.SRTP_AES128_CM_HMAC_SHA1_80,
		},
		VerifyPeerCertificate: func(raw [][]byte, _ [][]*x509.Certificate) error {
			if ep.remoteFP == "" || len(raw) == 0 {
				return nil
			}
			if !fpEqual(certFingerprintBytes(raw[0]), ep.remoteFP) {
				return fmt.Errorf("dtls fingerprint mismatch")
			}
			return nil
		},
	}
	conn, err := dtls.Client(vc, cfg)
	if err != nil {
		return
	}
	state := conn.ConnectionState()
	km, err := state.ExportKeyingMaterial("EXTRACTOR-dtls_srtp", nil, 60)
	if err != nil || len(km) < 60 {
		_ = conn.Close()
		return
	}
	// We are the DTLS client: client_write is local, server_write is remote.
	localKey, remoteKey := km[0:16], km[16:32]
	localSalt, remoteSalt := km[32:46], km[46:60]
	enc, err1 := srtp.CreateContext(localKey, localSalt, srtp.ProtectionProfileAes128CmHmacSha1_80)
	dec, err2 := srtp.CreateContext(remoteKey, remoteSalt, srtp.ProtectionProfileAes128CmHmacSha1_80)
	if err1 != nil || err2 != nil {
		_ = conn.Close()
		return
	}
	ep.dtls = conn
	ep.enc = enc
	ep.dec = dec
	ep.ready.Store(true)
}

type dgramConn struct {
	in     <-chan []byte
	stop   <-chan struct{}
	remote net.Addr
	write  func([]byte) error
	rest   []byte
}

func (c *dgramConn) Read(p []byte) (int, error) {
	if len(c.rest) > 0 {
		n := copy(p, c.rest)
		c.rest = c.rest[n:]
		return n, nil
	}
	select {
	case <-c.stop:
		return 0, io.EOF
	case pkt, ok := <-c.in:
		if !ok {
			return 0, io.EOF
		}
		n := copy(p, pkt)
		if n < len(pkt) {
			c.rest = append([]byte(nil), pkt[n:]...)
		}
		return n, nil
	}
}

func (c *dgramConn) Write(p []byte) (int, error) {
	if err := c.write(p); err != nil {
		return 0, err
	}
	return len(p), nil
}

func (c *dgramConn) Close() error                     { return nil }
func (c *dgramConn) LocalAddr() net.Addr              { return &net.UDPAddr{} }
func (c *dgramConn) RemoteAddr() net.Addr             { return c.remote }
func (c *dgramConn) SetDeadline(time.Time) error      { return nil }
func (c *dgramConn) SetReadDeadline(time.Time) error  { return nil }
func (c *dgramConn) SetWriteDeadline(time.Time) error { return nil }

func certFingerprint(cert tls.Certificate) string {
	if len(cert.Certificate) == 0 {
		return ""
	}
	return certFingerprintBytes(cert.Certificate[0])
}

func certFingerprintBytes(der []byte) string {
	sum := sha256.Sum256(der)
	var b strings.Builder
	for i, v := range sum {
		if i > 0 {
			b.WriteByte(':')
		}
		fmt.Fprintf(&b, "%02X", v)
	}
	return b.String()
}

func fpEqual(a, b string) bool {
	return strings.EqualFold(strip(a), strip(b))
}

func strip(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, ":", ""), " ", "")
}

func iceString(n int) string {
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	raw := make([]byte, n)
	_, _ = rand.Read(raw)
	for i := range b {
		b[i] = chars[int(raw[i])%len(chars)]
	}
	return string(b)
}
