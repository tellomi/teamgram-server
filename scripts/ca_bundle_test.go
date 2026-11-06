package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCABundleProbeRejectsEmptyTrustStore(t *testing.T) {
	script := probeScript(t)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "etc", "ssl", "certs"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("bash", script, root)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("empty /etc/ssl/certs was accepted: %s", out)
	}
	if !strings.Contains(string(out), "ca-certificates.crt") {
		t.Fatalf("output = %q", out)
	}
}

func TestCABundleProbeRejectsBundleWithoutPEM(t *testing.T) {
	script := probeScript(t)
	root := t.TempDir()
	dir := filepath.Join(root, "etc", "ssl", "certs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ca-certificates.crt"), []byte("not a certificate\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command("bash", script, root).CombinedOutput()
	if err == nil {
		t.Fatalf("non-PEM bundle was accepted: %s", out)
	}
	if !strings.Contains(string(out), "no PEM") {
		t.Fatalf("output = %q", out)
	}
}

func TestCABundleProbeAcceptsBundle(t *testing.T) {
	script := probeScript(t)
	root := t.TempDir()
	dir := filepath.Join(root, "etc", "ssl", "certs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pem := "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	if err := os.WriteFile(filepath.Join(dir, "ca-certificates.crt"), []byte(pem), 0o644); err != nil {
		t.Fatal(err)
	}
	// Host SSL_CERT_* must not make a good fixture fail.
	cmd := exec.Command("bash", script, root)
	cmd.Env = append(os.Environ(), "SSL_CERT_FILE=/no/such/bundle.crt", "SSL_CERT_DIR=/no/such/dir")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("probe rejected a bundle: %v\n%s", err, out)
	}
}

func TestDockerfileFinalStageInstallsCABundle(t *testing.T) {
	stage := finalDockerfileStage(t)
	if !strings.HasPrefix(strings.TrimSpace(stage), "FROM ubuntu:latest\n") {
		t.Fatalf("runtime base changed:\n%s", firstLine(stage))
	}
	if strings.Contains(stage, "AS ") {
		t.Fatalf("final stage must be the runtime image, not a named stage:\n%s", firstLine(stage))
	}
	if !strings.Contains(stage, "--no-install-recommends") || !strings.Contains(stage, "ca-certificates") {
		t.Fatal("runtime stage must install ca-certificates with --no-install-recommends")
	}
	if !strings.Contains(stage, "libvips42t64") || !strings.Contains(stage, "libvips42") {
		t.Fatal("libvips runtime package selection was dropped")
	}
	if !strings.Contains(stage, "/tmp/verify-ca-bundle.sh") {
		t.Fatal("image build does not run verify-ca-bundle.sh")
	}
	for _, banned := range []string{"InsecureSkipVerify", "SSL_CERT_FILE=", "SSL_CERT_DIR=", "http://"} {
		if strings.Contains(stage, banned) {
			t.Fatalf("runtime stage contains %q", banned)
		}
	}
}

func probeScript(t *testing.T) string {
	t.Helper()
	path := filepath.Join("verify-ca-bundle.sh")
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
	return path
}

func finalDockerfileStage(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "Dockerfile"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(body)
	start := strings.LastIndex(text, "\nFROM ")
	if start < 0 {
		t.Fatal("Dockerfile has no FROM")
	}
	return text[start+1:]
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return line
}
