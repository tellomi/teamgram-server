package mysqltls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
)

const sanName = "pc-2vc5itvsn950rd8w2.rwlb.cn-chengdu.rds.aliyuncs.com"

func TestApplyLeavesDSNUnchangedWhenUnset(t *testing.T) {
	t.Setenv(envCA, "")
	t.Setenv(envMode, "")
	t.Setenv(envServerName, "")
	in := "teamgram:s3cret@tcp(mysql:3306)/teamgram?charset=utf8mb4&parseTime=true&loc=Local"
	got, err := Apply(in)
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Fatalf("dsn changed to %q", got)
	}
}

func TestApplySetsTLSName(t *testing.T) {
	caPEM, _ := newCA(t)
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	if err := os.WriteFile(caPath, caPEM, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv(envCA, caPath)
	t.Setenv(envMode, modeVerifyCA)
	t.Setenv(envServerName, "")
	in := "teamgram:s3cret@tcp(hwsfbmvgttfh.rwlb.cn-chengdu.rds.aliyuncs.com:3306)/teamgram?charset=utf8mb4&parseTime=true&tls=true"
	got, err := Apply(in)
	if err != nil {
		t.Fatal(err)
	}
	if got == in {
		t.Fatal("dsn was left unchanged while the CA is set")
	}
	cfg, err := mysql.ParseDSN(got)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TLSConfig != ConfigName {
		t.Fatalf("tls config %q", cfg.TLSConfig)
	}
	if cfg.User != "teamgram" || cfg.Passwd != "s3cret" || cfg.DBName != "teamgram" || !cfg.ParseTime {
		t.Fatalf("dsn fields user=%s db=%s parseTime=%v", cfg.User, cfg.DBName, cfg.ParseTime)
	}
	if cfg.Addr != "hwsfbmvgttfh.rwlb.cn-chengdu.rds.aliyuncs.com:3306" {
		t.Fatalf("addr %s", cfg.Addr)
	}
}

func TestVerifyCAAcceptsMismatchedName(t *testing.T) {
	caPEM, caKey := newCA(t)
	leaf := issue(t, caPEM, caKey, sanName)
	cfg := mustBuild(t, caPEM, modeVerifyCA, "")
	if !cfg.InsecureSkipVerify {
		t.Fatal("verify-ca must skip the hostname check and verify the chain itself")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("min version %x", cfg.MinVersion)
	}
	if err := handshake(t, leaf, cfg); err != nil {
		t.Fatalf("mismatched dial name should pass verify-ca: %v", err)
	}
}

func TestVerifyCARejectsOtherCA(t *testing.T) {
	trustedPEM, _ := newCA(t)
	otherPEM, otherKey := newCA(t)
	leaf := issue(t, otherPEM, otherKey, sanName)
	cfg := mustBuild(t, trustedPEM, modeVerifyCA, "")
	if err := handshake(t, leaf, cfg); err == nil {
		t.Fatal("certificate from another CA was accepted")
	}
	self := selfSigned(t, sanName)
	if err := handshake(t, self, cfg); err == nil {
		t.Fatal("self-signed certificate was accepted")
	}
}

func TestVerifyFullServerName(t *testing.T) {
	caPEM, caKey := newCA(t)
	leaf := issue(t, caPEM, caKey, sanName)
	match := mustBuild(t, caPEM, modeVerifyFull, sanName)
	if match.InsecureSkipVerify {
		t.Fatal("verify-full must keep hostname verification")
	}
	if match.ServerName != sanName {
		t.Fatalf("ServerName %q", match.ServerName)
	}
	if err := handshake(t, leaf, match); err != nil {
		t.Fatalf("SAN match: %v", err)
	}
	miss := mustBuild(t, caPEM, "", "hwsfbmvgttfh.rwlb.cn-chengdu.rds.aliyuncs.com")
	if err := handshake(t, leaf, miss); err == nil {
		t.Fatal("public endpoint name was accepted as the certificate SAN")
	}
	other := mustBuild(t, caPEM, modeVerifyFull, "wrong.example")
	if err := handshake(t, leaf, other); err == nil {
		t.Fatal("unrelated ServerName was accepted")
	}
}

func TestBuildConfigRejectsUnknownMode(t *testing.T) {
	caPEM, _ := newCA(t)
	if _, err := BuildConfig(caPEM, "skip-verify", ""); err == nil {
		t.Fatal("skip-verify was accepted")
	}
}

func mustBuild(t *testing.T, caPEM []byte, mode, serverName string) *tls.Config {
	t.Helper()
	cfg, err := BuildConfig(caPEM, mode, serverName)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func handshake(t *testing.T, cert tls.Certificate, client *tls.Config) error {
	t.Helper()
	ln, err := tls.Listen("tcp", "127.0.0.1:0", &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	errCh := make(chan error, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			errCh <- err
			return
		}
		defer conn.Close()
		errCh <- conn.(*tls.Conn).Handshake()
	}()
	conn, err := tls.Dial("tcp", ln.Addr().String(), client)
	if err != nil {
		return err
	}
	conn.Close()
	return <-errCh
}

func newCA(t *testing.T) (pemBytes []byte, key *ecdsa.PrivateKey) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), key
}

func issue(t *testing.T, caPEM []byte, caKey *ecdsa.PrivateKey, dns string) tls.Certificate {
	t.Helper()
	ca, err := x509.ParseCertificate(pemDecode(t, caPEM))
	if err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: dns},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{dns},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca, &key.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func selfSigned(t *testing.T, dns string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(9),
		Subject:      pkix.Name{CommonName: dns},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{dns},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func pemDecode(t *testing.T, pemBytes []byte) []byte {
	t.Helper()
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		t.Fatal("no pem")
	}
	return block.Bytes
}
