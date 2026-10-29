package e2e

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gotd/td/exchange"
)

// Config is the checked-in test-account file (config.json). Environment
// variables override it for a staging or production smoke run.
type Config struct {
	Addr       string           `json:"addr"`
	APIID      int32            `json:"api_id"`
	APIHash    string           `json:"api_hash"`
	DC         int              `json:"dc"`
	RSAKeyFile string           `json:"rsa_key_file"`
	Phones     map[string]Phone `json:"phones"`
}

// Phone is a Telegram-style test number. Code is the fixed login code.
// For the built-in 99966XYYYY range the code is the datacenter digit five
// times (9996610001 -> 11111). The server accepts that code without SMS.
type Phone struct {
	Number string `json:"number"`
	Code   string `json:"code"`
}

// LoadConfig reads the embedded-adjacent config.json and applies overrides.
func LoadConfig() (Config, error) {
	path := os.Getenv("TELLOMI_E2E_CONFIG")
	if path == "" {
		path = filepath.Join(configDir(), "config.json")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read test config %s: %w", path, err)
	}
	var cfg Config
	if err := json.Unmarshal(raw, &cfg); err != nil {
		return Config{}, err
	}
	if v := os.Getenv("TELLOMI_E2E_ADDR"); v != "" {
		cfg.Addr = v
	}
	if v := os.Getenv("TELLOMI_E2E_API_ID"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("TELLOMI_E2E_API_ID: %w", err)
		}
		cfg.APIID = int32(n)
	}
	if v := os.Getenv("TELLOMI_E2E_API_HASH"); v != "" {
		cfg.APIHash = v
	}
	if v := os.Getenv("TELLOMI_E2E_DC"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return Config{}, fmt.Errorf("TELLOMI_E2E_DC: %w", err)
		}
		cfg.DC = n
	}
	if v := os.Getenv("TELLOMI_E2E_RSA_KEY"); v != "" {
		cfg.RSAKeyFile = v
	}
	if cfg.DC == 0 {
		cfg.DC = 1
	}
	for name, phone := range cfg.Phones {
		envNum := os.Getenv("TELLOMI_E2E_PHONE_" + strings.ToUpper(name))
		if envNum != "" {
			phone.Number = envNum
			if phone.Code == "" || os.Getenv("TELLOMI_E2E_CODE_"+strings.ToUpper(name)) == "" {
				phone.Code = TestPhoneCode(envNum)
			}
		}
		if v := os.Getenv("TELLOMI_E2E_CODE_" + strings.ToUpper(name)); v != "" {
			phone.Code = v
		}
		if phone.Code == "" {
			phone.Code = TestPhoneCode(phone.Number)
		}
		cfg.Phones[name] = phone
	}
	if cfg.Addr == "" || cfg.APIID == 0 || cfg.APIHash == "" {
		return Config{}, fmt.Errorf("test config is missing addr, api_id, or api_hash")
	}
	return cfg, nil
}

func (c Config) Phone(name string) (Phone, error) {
	p, ok := c.Phones[name]
	if !ok || p.Number == "" || p.Code == "" {
		return Phone{}, fmt.Errorf("test config has no phone %q", name)
	}
	return p, nil
}

func (c Config) PublicKey() (exchange.PublicKey, error) {
	path := c.RSAKeyFile
	if path == "" {
		path = filepath.Join(configDir(), "..", "teamgramd", "bin", "server_pkcs1.key")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(configDir(), path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		alt := filepath.Join(configDir(), "..", "teamgramd", "bin", "server_pkcs1.key")
		raw, err = os.ReadFile(alt)
		if err != nil {
			return exchange.PublicKey{}, fmt.Errorf("server RSA key: %w", err)
		}
	}
	block, _ := pem.Decode(raw)
	if block == nil {
		return exchange.PublicKey{}, fmt.Errorf("server RSA key is not PEM")
	}
	key, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		parsed, err2 := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err2 != nil {
			return exchange.PublicKey{}, err
		}
		priv, ok := parsed.(*rsa.PrivateKey)
		if !ok {
			return exchange.PublicKey{}, fmt.Errorf("server RSA key is not RSA")
		}
		key = priv
	}
	return exchange.PublicKey{RSA: &key.PublicKey}, nil
}

// TestPhoneCode matches the server default: the first digit after 99966,
// repeated five times.
func TestPhoneCode(phone string) string {
	digits := digitsOnly(phone)
	const prefix = "99966"
	if strings.HasPrefix(digits, prefix) && len(digits) > len(prefix) {
		return strings.Repeat(string(digits[len(prefix)]), 5)
	}
	return "12345"
}

func digitsOnly(phone string) string {
	var b strings.Builder
	for _, r := range phone {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func configDir() string {
	if root := os.Getenv("TELLOMI_E2E_ROOT"); root != "" {
		return filepath.Join(root, "e2e")
	}
	// go test runs with the package directory as the working directory.
	if st, err := os.Stat("config.json"); err == nil && !st.IsDir() {
		return "."
	}
	return "e2e"
}
