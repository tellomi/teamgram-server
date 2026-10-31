package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeromicro/go-zero/core/conf"
)

func TestRTMPURLDefault(t *testing.T) {
	t.Setenv("TELLOMI_RTMP_URL", "")
	got := (Config{}).Normalized().RTMPURL
	if got != "rtmp://127.0.0.1:1935/live" {
		t.Fatalf("default RTMPURL = %q", got)
	}
}

func TestRTMPURLKeepsYAMLWhenEnvUnset(t *testing.T) {
	t.Setenv("TELLOMI_RTMP_URL", "")
	got := (Config{RTMPURL: "rtmp://127.0.0.1:1935/live"}).Normalized().RTMPURL
	if got != "rtmp://127.0.0.1:1935/live" {
		t.Fatalf("yaml RTMPURL = %q", got)
	}
}

func TestRTMPURLEnvOverridesYAML(t *testing.T) {
	t.Setenv("TELLOMI_RTMP_URL", "rtmp://ingest.tellomi.app/live")
	got := (Config{RTMPURL: "rtmp://127.0.0.1:1935/live"}).Normalized().RTMPURL
	if got != "rtmp://ingest.tellomi.app/live" {
		t.Fatalf("env RTMPURL = %q", got)
	}
}

func TestIPv4IsNotAdvertisedAsIPv6(t *testing.T) {
	t.Setenv("TELLOMI_CALL_IP", "")
	t.Setenv("TELLOMI_CALL_IPV6", "")
	got := (Config{PublicIP: "8.137.39.82", PublicIPv6: "8.137.39.82"}).Normalized()
	if got.PublicIP != "8.137.39.82" || got.IPv6() != "" {
		t.Fatalf("ip %q ipv6 %q", got.PublicIP, got.IPv6())
	}
}

func TestTurnEnvOverridesYAML(t *testing.T) {
	t.Setenv("TELLOMI_CALL_IP", "8.137.39.82")
	t.Setenv("TELLOMI_CALL_IPV6", "2001:db8::10")
	t.Setenv("TELLOMI_TURN_HOST", "turn.tellomi.cn")
	t.Setenv("TELLOMI_TURN_PORT", "3478")
	t.Setenv("TELLOMI_STUN_HOST", "stun.tellomi.cn")
	t.Setenv("TELLOMI_STUN_PORT", "19302")
	t.Setenv("TELLOMI_TURN_SECRET", "chengdu-secret")
	t.Setenv("TELLOMI_TURN_TTL", "120")
	t.Setenv("TELLOMI_TURN_SERVERS", "198.51.100.8:3478, 198.51.100.9:3479")
	t.Setenv("TELLOMI_STUN_SERVERS", "[2001:db8::5]:3478, not-a-server, stun.tellomi.cn:19302")
	t.Setenv("TELLOMI_ENABLE_P2P", "false")
	t.Setenv("TELLOMI_CALL_FORCE_TCP", "true")
	got := (Config{
		PublicIP: "127.0.0.1", TurnHost: "127.0.0.1", TurnSecret: "tellomi-turn-secret", EnableP2P: true,
	}).Normalized()
	if got.PublicIP != "8.137.39.82" || got.IPv6() != "2001:db8::10" {
		t.Fatalf("addr ip=%s ipv6=%s", got.PublicIP, got.IPv6())
	}
	turns := got.TURNEndpoints()
	if len(turns) != 2 || turns[0].Host != "198.51.100.8" || turns[0].Port != 3478 || turns[1].Port != 3479 {
		t.Fatalf("turn %+v", turns)
	}
	stuns := got.STUNEndpoints()
	if len(stuns) != 2 || stuns[0].Host != "2001:db8::5" || stuns[1].Host != "stun.tellomi.cn" || stuns[1].Port != 19302 {
		t.Fatalf("stun %+v", stuns)
	}
	if got.TurnSecret != "chengdu-secret" || got.TurnTTL != 120 || got.EnableP2P || !got.ForceTCP {
		t.Fatalf("flags %+v", got)
	}
	body := got.CallConfigBody()
	if body == "{}" || !strings.Contains(body, `"enable_p2p":false`) || !strings.Contains(body, `"force_tcp":true`) {
		t.Fatal(body)
	}
	if strings.Contains(body, `"enable_p2p":true`) || strings.Contains(body, `"force_tcp":false`) {
		t.Fatal(body)
	}
}

func TestTurnPortZeroDisablesRelay(t *testing.T) {
	t.Setenv("TELLOMI_TURN_PORT", "0")
	t.Setenv("TELLOMI_STUN_PORT", "0")
	t.Setenv("TELLOMI_TURN_SERVERS", "")
	t.Setenv("TELLOMI_STUN_SERVERS", "")
	got := (Config{TurnHost: "10.0.0.1", StunHost: "10.0.0.1", TurnPort: 3478, StunPort: 3478}).Normalized()
	if len(got.TURNEndpoints()) != 0 || len(got.STUNEndpoints()) != 0 {
		t.Fatalf("turn %+v stun %+v", got.TURNEndpoints(), got.STUNEndpoints())
	}
}

func TestSingleTurnHostWhenListUnset(t *testing.T) {
	t.Setenv("TELLOMI_TURN_HOST", "")
	t.Setenv("TELLOMI_TURN_SERVERS", "")
	t.Setenv("TELLOMI_STUN_HOST", "")
	t.Setenv("TELLOMI_STUN_SERVERS", "")
	t.Setenv("TELLOMI_TURN_PORT", "")
	t.Setenv("TELLOMI_STUN_PORT", "")
	got := (Config{TurnHost: "turn.internal", TurnPort: 3478, StunPort: 19302}).Normalized()
	turns := got.TURNEndpoints()
	stuns := got.STUNEndpoints()
	if len(turns) != 1 || turns[0].Host != "turn.internal" || turns[0].Port != 3478 {
		t.Fatalf("turn %+v", turns)
	}
	if len(stuns) != 1 || stuns[0].Host != "turn.internal" || stuns[0].Port != 19302 {
		t.Fatalf("stun %+v", stuns)
	}
	body := (Config{EnableP2P: true}).CallConfigBody()
	if body == "{}" || !strings.Contains(body, `"enable_p2p":true`) || !strings.Contains(body, `"audio_frame_size":60`) {
		t.Fatal(body)
	}
}

func TestEmptyHostsStayLoopbackWithoutEnv(t *testing.T) {
	clearCallHostEnv(t)
	got := (Config{}).Normalized()
	if got.PublicIP != "127.0.0.1" || got.TurnHost != "127.0.0.1" || got.StunHost != "127.0.0.1" {
		t.Fatalf("ip=%s turn=%s stun=%s", got.PublicIP, got.TurnHost, got.StunHost)
	}
}

func TestTurnEnvFillsEmptyStunHost(t *testing.T) {
	clearCallHostEnv(t)
	t.Setenv("TELLOMI_TURN_HOST", "8.137.39.82")
	got := (Config{}).Normalized()
	if got.PublicIP != "127.0.0.1" || got.TurnHost != "8.137.39.82" || got.StunHost != "8.137.39.82" {
		t.Fatalf("ip=%s turn=%s stun=%s", got.PublicIP, got.TurnHost, got.StunHost)
	}
	stuns := got.STUNEndpoints()
	if len(stuns) != 1 || stuns[0].Host != "8.137.39.82" || stuns[0].Port != 3478 {
		t.Fatalf("stun %+v", stuns)
	}
}

func TestCallIPFillsEmptyTurnAndStun(t *testing.T) {
	clearCallHostEnv(t)
	t.Setenv("TELLOMI_CALL_IP", "8.137.39.82")
	got := (Config{}).Normalized()
	if got.PublicIP != "8.137.39.82" || got.TurnHost != "8.137.39.82" || got.StunHost != "8.137.39.82" {
		t.Fatalf("ip=%s turn=%s stun=%s", got.PublicIP, got.TurnHost, got.StunHost)
	}
}

func TestExplicitLoopbackStunHostIsNotReplacedByTurnEnv(t *testing.T) {
	clearCallHostEnv(t)
	t.Setenv("TELLOMI_TURN_HOST", "8.137.39.82")
	got := (Config{StunHost: "127.0.0.1"}).Normalized()
	if got.TurnHost != "8.137.39.82" || got.StunHost != "127.0.0.1" {
		t.Fatalf("turn=%s stun=%s", got.TurnHost, got.StunHost)
	}
}

func TestShippedPhoneYAMLLoadsEmptyHosts(t *testing.T) {
	clearCallHostEnv(t)
	files := []string{
		"../../../../../teamgramd/etc/bff.yaml",
		"../../../../../teamgramd/etc2/bff.yaml",
		"../../../../bff/bff/etc/bff.yaml",
	}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		block := phoneBlock(string(body))
		dir := t.TempDir()
		snippet := filepath.Join(dir, "phone.yaml")
		if err := os.WriteFile(snippet, []byte(block), 0o644); err != nil {
			t.Fatal(err)
		}
		var loaded struct {
			Phone Config `json:",optional"`
		}
		if err := conf.Load(snippet, &loaded); err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
		if loaded.Phone.PublicIP != "" || loaded.Phone.TurnHost != "" || loaded.Phone.StunHost != "" {
			t.Fatalf("%s loaded ip=%q turn=%q stun=%q", path, loaded.Phone.PublicIP, loaded.Phone.TurnHost, loaded.Phone.StunHost)
		}
		got := loaded.Phone.Normalized()
		if got.PublicIP != "127.0.0.1" || got.TurnHost != "127.0.0.1" || got.StunHost != "127.0.0.1" {
			t.Fatalf("%s normalized ip=%s turn=%s stun=%s", path, got.PublicIP, got.TurnHost, got.StunHost)
		}
	}
}

func TestShippedPhoneYAMLDoesNotPinLoopbackICE(t *testing.T) {
	files := []string{
		"../../../../../teamgramd/etc/bff.yaml",
		"../../../../../teamgramd/etc2/bff.yaml",
		"../../../../bff/bff/etc/bff.yaml",
	}
	for _, path := range files {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		block := phoneBlock(string(body))
		if block == "" {
			t.Fatalf("%s has no Phone block", path)
		}
		for _, key := range []string{"PublicIP", "TurnHost", "StunHost"} {
			val, ok := yamlScalar(block, key)
			if ok && isLoopbackHost(val) {
				t.Fatalf("%s Phone.%s = %q; leave it empty so STUN follows TURN", path, key, val)
			}
		}
	}
}

func clearCallHostEnv(t *testing.T) {
	t.Helper()
	for _, key := range []string{
		"TELLOMI_CALL_IP", "TELLOMI_CALL_IPV6",
		"TELLOMI_TURN_HOST", "TELLOMI_TURN_PORT", "TELLOMI_TURN_SERVERS",
		"TELLOMI_STUN_HOST", "TELLOMI_STUN_PORT", "TELLOMI_STUN_SERVERS",
	} {
		t.Setenv(key, "")
	}
}

func phoneBlock(s string) string {
	const marker = "\nPhone:"
	i := strings.Index(s, marker)
	if i < 0 {
		if strings.HasPrefix(s, "Phone:") {
			i = -1
		} else {
			return ""
		}
	}
	rest := s[i+1:]
	lines := strings.Split(rest, "\n")
	var b strings.Builder
	b.WriteString(lines[0])
	b.WriteByte('\n')
	for _, line := range lines[1:] {
		if line != "" && line[0] != ' ' && line[0] != '\t' && line[0] != '#' {
			break
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

func yamlScalar(block, key string) (string, bool) {
	prefix := key + ":"
	for _, line := range strings.Split(block, "\n") {
		trim := strings.TrimSpace(line)
		if trim == "" || strings.HasPrefix(trim, "#") {
			continue
		}
		if !strings.HasPrefix(trim, prefix) {
			continue
		}
		val := strings.TrimSpace(strings.TrimPrefix(trim, prefix))
		if i := strings.Index(val, " #"); i >= 0 {
			val = strings.TrimSpace(val[:i])
		}
		val = strings.Trim(val, `"'`)
		return val, true
	}
	return "", false
}

func isLoopbackHost(host string) bool {
	switch strings.ToLower(host) {
	case "127.0.0.1", "localhost", "::1", "[::1]":
		return true
	default:
		return false
	}
}
