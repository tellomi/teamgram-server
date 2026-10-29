package config

import "testing"

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
