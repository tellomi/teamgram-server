package gnet

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/teamgram/proto/mtproto/crypto"
	"github.com/teamgram/teamgram-server/app/interface/gnetway/internal/config"
)

// Public fingerprints. The private production key is not in git.
// See docs/SECURITY-OPS.md.
const (
	upstreamRSAFingerprint uint64 = 12240908862933197005
	rotatedRSAFingerprint  uint64 = 4381149949997195609
)

func TestUpstreamKeyFingerprint(t *testing.T) {
	if upstreamRSAFingerprint == rotatedRSAFingerprint {
		t.Fatal("production fingerprint must not be the upstream default")
	}
	for _, rel := range []string{
		"teamgramd/bin/server_pkcs1.key",
		"proto/mtproto/crypto/test_pkcs1.key",
	} {
		path := repoPath(t, rel)
		key, err := crypto.NewRSACryptor(path)
		if err != nil {
			t.Fatal(err)
		}
		if got := rsaKeyFingerprint(key); got != upstreamRSAFingerprint {
			t.Fatalf("%s fingerprint %d, want %d", rel, got, upstreamRSAFingerprint)
		}
	}
}

func TestMustNewHandshakeChecksFingerprint(t *testing.T) {
	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FILE", "")
	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT", "")
	path := repoPath(t, "teamgramd/bin/server_pkcs1.key")

	h := mustNewHandshake([]config.RSAKey{{
		KeyFile:        path,
		KeyFingerprint: strconv.FormatUint(upstreamRSAFingerprint, 10),
	}})
	if len(h.keyFingerprints) != 1 || uint64(h.keyFingerprints[0]) != upstreamRSAFingerprint {
		t.Fatalf("fingerprints %#v", h.keyFingerprints)
	}

	mustPanic(t, func() {
		mustNewHandshake([]config.RSAKey{{
			KeyFile:        path,
			KeyFingerprint: "1",
		}})
	})
}

func TestMustNewHandshakePrependsExtraKey(t *testing.T) {
	extraPath, extraFP := writeTempRSAKey(t)
	upstream := repoPath(t, "teamgramd/bin/server_pkcs1.key")
	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FILE", extraPath)
	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT", strconv.FormatUint(extraFP, 10))

	h := mustNewHandshake([]config.RSAKey{{
		KeyFile:        upstream,
		KeyFingerprint: strconv.FormatUint(upstreamRSAFingerprint, 10),
	}})
	if len(h.keyFingerprints) != 2 {
		t.Fatalf("len %d", len(h.keyFingerprints))
	}
	if uint64(h.keyFingerprints[0]) != extraFP || uint64(h.keyFingerprints[1]) != upstreamRSAFingerprint {
		t.Fatalf("order %#v, want extra %d then upstream %d", h.keyFingerprints, extraFP, upstreamRSAFingerprint)
	}
	upstreamFP := upstreamRSAFingerprint
	if h.getKey(int64(extraFP)) == nil || h.getKey(int64(upstreamFP)) == nil {
		t.Fatal("key missing from rsa list")
	}
}

func TestMustNewHandshakeRejectsHalfSetExtraKey(t *testing.T) {
	path := repoPath(t, "teamgramd/bin/server_pkcs1.key")
	keys := []config.RSAKey{{
		KeyFile:        path,
		KeyFingerprint: strconv.FormatUint(upstreamRSAFingerprint, 10),
	}}

	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FILE", path)
	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT", "")
	mustPanic(t, func() { mustNewHandshake(keys) })

	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FILE", "")
	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT", strconv.FormatUint(upstreamRSAFingerprint, 10))
	mustPanic(t, func() { mustNewHandshake(keys) })
}

func TestMustNewHandshakeRejectsExtraKeyFingerprintMismatch(t *testing.T) {
	extraPath, _ := writeTempRSAKey(t)
	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FILE", extraPath)
	t.Setenv("TELLOMI_MTPROTO_EXTRA_KEY_FINGERPRINT", "1")
	mustPanic(t, func() {
		mustNewHandshake(nil)
	})
}

func writeTempRSAKey(t *testing.T) (string, uint64) {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	raw := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})
	path := filepath.Join(t.TempDir(), "extra.pkcs1.key")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	key, err := crypto.NewRSACryptor(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, rsaKeyFingerprint(key)
}

func repoPath(t *testing.T, rel string) string {
	t.Helper()
	path := filepath.Join("..", "..", "..", "..", "..", "..", rel)
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func mustPanic(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
