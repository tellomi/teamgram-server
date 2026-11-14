package sfu

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"crypto/tls"
	"github.com/pion/dtls/v2"
	"github.com/pion/dtls/v2/pkg/crypto/selfsign"
	"github.com/pion/stun"
)

// Mimics tgcalls GroupNetworkManager: DTLS server, SRTP GCM only (aes128_sha1_80 disabled).
func TestTgcallsGCMOnlyDTLSHandshake(t *testing.T) {
	s, err := Listen("127.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	cert, _ := selfsign.GenerateSelfSigned()
	fp := certFingerprintBytes(cert.Certificate[0])
	offer := fmt.Sprintf(`{"ssrc":77,"ufrag":"cu","pwd":"client-password-value","fingerprints":[{"hash":"sha-256","setup":"passive","fingerprint":"%s"}]}`, fp)
	answer, _, _, err := s.Join(1, "k", offer, "1")
	if err != nil {
		t.Fatal(err)
	}
	var a struct{ Transport struct{ Ufrag, Pwd string } }
	_ = json.Unmarshal([]byte(answer), &a)
	c, _ := net.DialUDP("udp", nil, s.conn.LocalAddr().(*net.UDPAddr))
	req, _ := stun.Build(stun.BindingRequest, stun.NewUsername(a.Transport.Ufrag+":cu"), stun.NewShortTermIntegrity(a.Transport.Pwd), stun.Fingerprint)
	_, _ = c.Write(req.Raw)
	// filter STUN out of the DTLS stream
	pc := &stunFilter{c}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	srv, err := dtls.ServerWithContext(ctx, pc, &dtls.Config{
		Certificates:           []tls.Certificate{cert},
		SRTPProtectionProfiles: []dtls.SRTPProtectionProfile{dtls.SRTP_AEAD_AES_128_GCM, dtls.SRTP_AEAD_AES_256_GCM},
		ClientAuth:             dtls.RequireAnyClientCert,
		InsecureSkipVerify:     true,
	})
	if err != nil {
		t.Fatalf("tgcalls-like DTLS server (GCM only) handshake failed: %v", err)
	}
	defer srv.Close()
	p, _ := srv.SelectedSRTPProtectionProfile()
	if p != dtls.SRTP_AEAD_AES_128_GCM && p != dtls.SRTP_AEAD_AES_256_GCM {
		t.Fatalf("profile=%v, want GCM", p)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		s.mu.Lock()
		eps := append([]*endpoint(nil), s.calls[1]...)
		s.mu.Unlock()
		if len(eps) == 1 && eps[0].ready.Load() && eps[0].enc != nil && eps[0].dec != nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("SFU endpoint did not become ready after GCM handshake")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

type stunFilter struct{ *net.UDPConn }

func (f *stunFilter) Read(b []byte) (int, error) {
	for {
		n, err := f.UDPConn.Read(b)
		if err != nil || n == 0 || b[0] >= 2 {
			return n, err
		}
	}
}
