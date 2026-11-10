package loopback

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoopbackOverlayBindsHostPortsToLoopback(t *testing.T) {
	root := repoRoot(t)
	overlay := filepath.Join(root, "e2e", "docker-compose.loopback.yaml")
	text, err := os.ReadFile(overlay)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(text), "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- ") && (strings.Contains(trim, "0.0.0.0:") || strings.HasPrefix(trim, `- "`)) && !strings.Contains(trim, "127.0.0.1:") {
			t.Fatalf("loopback overlay publishes a wildcard host port: %s", trim)
		}
	}
	if _, err = exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	ports := teamgramPorts(t, root, []string{
		"docker-compose-env.yaml",
		"docker-compose.yaml",
		"e2e/docker-compose.loopback.yaml",
	})
	if len(ports) == 0 {
		t.Fatal("merged config dropped teamgram ports")
	}
	saw10443 := false
	saw10444 := false
	for _, port := range ports {
		if port.HostIP != "127.0.0.1" {
			t.Fatalf("teamgram port published on %q", port.HostIP)
		}
		if portNumber(port.Published) == "10443" {
			saw10443 = true
		}
		if portNumber(port.Published) == "10444" && portNumber(port.Target) == "10444" {
			saw10444 = true
		}
	}
	if !saw10443 {
		t.Fatal("merged config dropped 10443")
	}
	if !saw10444 {
		t.Fatal("loopback overlay dropped private 10444")
	}
}

func TestPrivate10444PublishedOnLoopback(t *testing.T) {
	root := repoRoot(t)
	base, err := os.ReadFile(filepath.Join(root, "docker-compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !portLine(string(base), `"127.0.0.1:10444:10444"`) {
		t.Fatal("docker-compose.yaml does not publish 127.0.0.1:10444:10444")
	}
	if portLine(string(base), `"10444:10444"`) {
		t.Fatal("docker-compose.yaml publishes 10444 on all interfaces")
	}
	overlay, err := os.ReadFile(filepath.Join(root, "e2e", "docker-compose.loopback.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(overlay), `"127.0.0.1:10444:10444"`) {
		t.Fatal("loopback overlay dropped 127.0.0.1:10444:10444")
	}
	if _, err = exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	for _, files := range [][]string{
		{"docker-compose-env.yaml", "docker-compose.yaml"},
		{"docker-compose-env.yaml", "docker-compose.yaml", "e2e/docker-compose.loopback.yaml"},
	} {
		var found bool
		for _, port := range teamgramPorts(t, root, files) {
			if portNumber(port.Target) != "10444" {
				continue
			}
			found = true
			if port.HostIP != "127.0.0.1" {
				t.Fatalf("%v publishes container 10444 on %q", files, port.HostIP)
			}
			if portNumber(port.Published) != "10444" {
				t.Fatalf("%v published 10444 as %v", files, port.Published)
			}
		}
		if !found {
			t.Fatalf("%v does not publish container port 10444", files)
		}
	}
}

type composePort struct {
	Published any    `json:"published"`
	HostIP    string `json:"host_ip"`
	Target    any    `json:"target"`
}

func portLine(text, quoted string) bool {
	for _, line := range strings.Split(text, "\n") {
		trim := strings.TrimSpace(line)
		if strings.HasPrefix(trim, "- ") && strings.Contains(trim, quoted) {
			return true
		}
	}
	return false
}

func portNumber(v any) string {
	switch n := v.(type) {
	case string:
		return n
	case float64:
		if n == float64(int(n)) {
			return itoa(int(n))
		}
	}
	return ""
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [8]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Clean(filepath.Join(wd, "..", ".."))
}

func teamgramPorts(t *testing.T, root string, files []string) []composePort {
	t.Helper()
	args := []string{"compose"}
	for _, f := range files {
		args = append(args, "-f", filepath.Join(root, f))
	}
	args = append(args, "config", "--format", "json")
	cmd := exec.Command("docker", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compose config: %v\n%s", err, out)
	}
	var cfg struct {
		Services map[string]struct {
			Ports []composePort `json:"ports"`
		} `json:"services"`
	}
	if err = json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	return cfg.Services["teamgram"].Ports
}
