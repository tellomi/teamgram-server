package push

import (
	"os"
	"strings"
)

const (
	EnvKeyID   = "TELLOMI_APNS_KEY_ID"
	EnvTeamID  = "TELLOMI_APNS_TEAM_ID"
	EnvTopic   = "TELLOMI_APNS_TOPIC"
	EnvKeyPath = "TELLOMI_APNS_KEY_PATH"
	EnvSandbox = "TELLOMI_APNS_SANDBOX"

	productionHost = "https://api.push.apple.com"
	sandboxHost    = "https://api.sandbox.push.apple.com"
)

// Config is the APNs token-auth (.p8) configuration.
// TestFlight and App Store use the production host. Set Sandbox only for
// Xcode debug builds, or leave it false and let each device's app_sandbox flag
// select the host.
type Config struct {
	KeyID   string
	TeamID  string
	Topic   string
	KeyPath string
	Sandbox bool
	KeyPEM  []byte // optional in-memory key, used by tests
}

func LoadConfig() Config {
	return Config{
		KeyID:   strings.TrimSpace(os.Getenv(EnvKeyID)),
		TeamID:  strings.TrimSpace(os.Getenv(EnvTeamID)),
		Topic:   strings.TrimSpace(os.Getenv(EnvTopic)),
		KeyPath: strings.TrimSpace(os.Getenv(EnvKeyPath)),
		Sandbox: envBool(os.Getenv(EnvSandbox)),
	}
}

// Enabled reports whether enough APNs settings are present to send.
func (c Config) Enabled() bool {
	if c.KeyID == "" || c.TeamID == "" || c.Topic == "" {
		return false
	}
	return c.KeyPath != "" || len(c.KeyPEM) > 0
}

func (c Config) host(deviceSandbox bool) string {
	if c.Sandbox || deviceSandbox {
		return sandboxHost
	}
	return productionHost
}

func envBool(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
