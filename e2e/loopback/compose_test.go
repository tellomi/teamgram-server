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
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Clean(filepath.Join(wd, "..", ".."))
	overlay := filepath.Join(root, "e2e", "docker-compose.loopback.yaml")
	base := filepath.Join(root, "docker-compose.yaml")
	env := filepath.Join(root, "docker-compose-env.yaml")
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
	cmd := exec.Command("docker", "compose", "-f", env, "-f", base, "-f", overlay, "config", "--format", "json")
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("compose config: %v\n%s", err, out)
	}
	var cfg struct {
		Services map[string]struct {
			Ports []struct {
				Published any    `json:"published"`
				HostIP    string `json:"host_ip"`
			} `json:"ports"`
		} `json:"services"`
	}
	if err = json.Unmarshal(out, &cfg); err != nil {
		t.Fatal(err)
	}
	ports := cfg.Services["teamgram"].Ports
	if len(ports) == 0 {
		t.Fatal("merged config dropped teamgram ports")
	}
	saw10443 := false
	for _, port := range ports {
		if port.HostIP != "127.0.0.1" {
			t.Fatalf("teamgram port published on %q", port.HostIP)
		}
		if port.Published == "10443" || port.Published == float64(10443) {
			saw10443 = true
		}
	}
	if !saw10443 {
		t.Fatal("merged config dropped 10443")
	}
}
