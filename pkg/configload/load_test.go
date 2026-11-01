package configload

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zeromicro/go-zero/core/service"
)

func TestOtelEndpointExpandsOrDisables(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "svc.yaml")
	body := []byte("Name: svc\nTelemetry:\n  Name: svc\n  Endpoint: ${TELLOMI_OTEL_ENDPOINT}\n  Sampler: 1.0\n  Batcher: otlpgrpc\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("unset", func(t *testing.T) {
		t.Setenv("TELLOMI_OTEL_ENDPOINT", "")
		var c service.ServiceConf
		MustLoad(path, &c)
		if c.Telemetry.Endpoint != "" || !c.Telemetry.Disabled {
			t.Fatalf("endpoint %q disabled %v, want empty and disabled", c.Telemetry.Endpoint, c.Telemetry.Disabled)
		}
	})

	t.Run("set", func(t *testing.T) {
		t.Setenv("TELLOMI_OTEL_ENDPOINT", "jaeger:4317")
		var c service.ServiceConf
		MustLoad(path, &c)
		if c.Telemetry.Endpoint != "jaeger:4317" || c.Telemetry.Disabled {
			t.Fatalf("endpoint %q disabled %v, want jaeger:4317 enabled", c.Telemetry.Endpoint, c.Telemetry.Disabled)
		}
	})
}

func TestHardcodedEndpointStays(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "svc.yaml")
	body := []byte("Name: svc\nTelemetry:\n  Name: svc\n  Endpoint: jaeger:4317\n  Batcher: otlpgrpc\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TELLOMI_OTEL_ENDPOINT", "")
	var c service.ServiceConf
	MustLoad(path, &c)
	if c.Telemetry.Endpoint != "jaeger:4317" || c.Telemetry.Disabled {
		t.Fatalf("endpoint %q disabled %v", c.Telemetry.Endpoint, c.Telemetry.Disabled)
	}
}

func TestEtc2PlaceholderIsWhatTheLoaderExpands(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "teamgramd", "etc2", "bff.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "Endpoint: ${TELLOMI_OTEL_ENDPOINT}") {
		t.Fatal("etc2/bff.yaml no longer has the placeholder the loader expands")
	}
}
