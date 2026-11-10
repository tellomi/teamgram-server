// Package mysqltls adds an opt-in TLS mode for every MySQL DSN this process opens.
//
// PolarDB's server certificate is signed by the operator CA, but its SAN is the
// cluster's private endpoint (for the Chengdu cluster,
// pc-2vc5itvsn950rd8w2.rwlb.cn-chengdu.rds.aliyuncs.com), not the public
// hostname clients dial. Go's default tls=true checks that public hostname and
// fails. VERIFY_CA succeeds because the chain is valid.
//
// When TEAMGRAM_MYSQL_TLS_CA is unset, DSNs are not modified and no tls.Config
// is registered. When it is set, a config is registered under the name
// "teamgram" and DSNs gain tls=teamgram.
//
//	TEAMGRAM_MYSQL_TLS_CA          PEM bundle of the CAs that sign the server cert.
//	TEAMGRAM_MYSQL_TLS_MODE        verify-full (default) or verify-ca.
//	TEAMGRAM_MYSQL_TLS_SERVER_NAME DNS name to check. verify-full only.
//
// verify-full keeps hostname verification on. Set TEAMGRAM_MYSQL_TLS_SERVER_NAME
// to a name in the certificate SAN so the public endpoint can be dialed while
// the check still runs. verify-ca is the explicit fallback: InsecureSkipVerify
// is set only together with VerifyPeerCertificate, which checks the chain
// against the CA pool and does not check the hostname. MinVersion is TLS 1.2
// in both modes.
//
// The mysql CLI used by teamgramd/deploy/migrate.sh cannot set a TLS ServerName,
// so when the CA is set it connects with --ssl-mode=VERIFY_CA. That is weaker
// than verify-full and is intentional.
package mysqltls

import (
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/go-sql-driver/mysql"
	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

const (
	// ConfigName is the mysql driver TLS config DSNs reference as tls=.
	ConfigName = "teamgram"

	envCA         = "TEAMGRAM_MYSQL_TLS_CA"
	envMode       = "TEAMGRAM_MYSQL_TLS_MODE"
	envServerName = "TEAMGRAM_MYSQL_TLS_SERVER_NAME"

	modeVerifyFull = "verify-full"
	modeVerifyCA   = "verify-ca"
)

var (
	regOnce sync.Once
	regErr  error
)

// Apply returns dsn unchanged when TEAMGRAM_MYSQL_TLS_CA is unset.
// Otherwise it registers the TLS config and sets tls=teamgram on dsn.
func Apply(dsn string) (string, error) {
	if strings.TrimSpace(os.Getenv(envCA)) == "" {
		return dsn, nil
	}
	if err := register(); err != nil {
		return "", err
	}
	return withTLSName(dsn, ConfigName)
}

// Open is sql.Open("mysql", ...) after Apply.
func Open(dsn string) (*sql.DB, error) {
	dsn, err := Apply(dsn)
	if err != nil {
		return nil, err
	}
	return sql.Open("mysql", dsn)
}

// Prepare copies c and applies TLS to DSN and ReadDSN.
// A nil config is returned as nil.
func Prepare(c *sqlx.Config) (*sqlx.Config, error) {
	if c == nil {
		return nil, nil
	}
	cp := *c
	var err error
	cp.DSN, err = Apply(c.DSN)
	if err != nil {
		return nil, err
	}
	if len(c.ReadDSN) > 0 {
		cp.ReadDSN = make([]string, len(c.ReadDSN))
		for i, dsn := range c.ReadDSN {
			cp.ReadDSN[i], err = Apply(dsn)
			if err != nil {
				return nil, err
			}
		}
	}
	return &cp, nil
}

// NewMySQL is sqlx.NewMySQL after Prepare. A TLS setup error panics, which is
// the same failure mode sqlx.NewMySQL uses when the DSN cannot be opened.
func NewMySQL(c *sqlx.Config) *sqlx.DB {
	cp, err := Prepare(c)
	if err != nil {
		panic(err)
	}
	return sqlx.NewMySQL(cp)
}

// BuildConfig builds the tls.Config Apply registers.
// caPEM is a PEM bundle. mode is verify-full, verify-ca, or empty (verify-full).
// serverName is the verify-full ServerName override; empty keeps the driver's
// default of the dialed host.
func BuildConfig(caPEM []byte, mode, serverName string) (*tls.Config, error) {
	mode, err := normalizeMode(mode)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, errors.New("mysql tls: CA PEM contains no certificates")
	}
	cfg := &tls.Config{
		RootCAs:    pool,
		MinVersion: tls.VersionTLS12,
		ServerName: strings.TrimSpace(serverName),
	}
	if mode == modeVerifyCA {
		// InsecureSkipVerify disables the hostname check. VerifyPeerCertificate
		// still runs and is the only chain check, because the verifiedChains
		// argument is nil when InsecureSkipVerify is set.
		cfg.InsecureSkipVerify = true
		cfg.VerifyPeerCertificate = func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			return verifyChain(pool, rawCerts)
		}
	}
	return cfg, nil
}

func register() error {
	regOnce.Do(func() {
		regErr = registerFromEnv()
	})
	return regErr
}

func registerFromEnv() error {
	path := strings.TrimSpace(os.Getenv(envCA))
	pem, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("mysql tls: read %s: %w", envCA, err)
	}
	cfg, err := BuildConfig(pem, os.Getenv(envMode), os.Getenv(envServerName))
	if err != nil {
		return err
	}
	// RegisterTLSConfig replaces an existing entry. The name is not one of
	// the driver's reserved values (true, false, skip-verify, preferred).
	if err = mysql.RegisterTLSConfig(ConfigName, cfg); err != nil {
		return fmt.Errorf("mysql tls: register %s: %w", ConfigName, err)
	}
	return nil
}

func normalizeMode(mode string) (string, error) {
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" {
		return modeVerifyFull, nil
	}
	switch mode {
	case modeVerifyFull, modeVerifyCA:
		return mode, nil
	default:
		return "", fmt.Errorf("mysql tls: %s=%q, want verify-full or verify-ca", envMode, mode)
	}
}

func verifyChain(roots *x509.CertPool, rawCerts [][]byte) error {
	if len(rawCerts) == 0 {
		return errors.New("mysql tls: server presented no certificate")
	}
	certs := make([]*x509.Certificate, len(rawCerts))
	for i, asn1 := range rawCerts {
		c, err := x509.ParseCertificate(asn1)
		if err != nil {
			return fmt.Errorf("mysql tls: parse server certificate: %w", err)
		}
		certs[i] = c
	}
	inter := x509.NewCertPool()
	for _, c := range certs[1:] {
		inter.AddCert(c)
	}
	// DNSName left empty: the chain is checked and the SAN is not.
	if _, err := certs[0].Verify(x509.VerifyOptions{
		Roots:         roots,
		Intermediates: inter,
	}); err != nil {
		return fmt.Errorf("mysql tls: certificate chain: %w", err)
	}
	return nil
}

func withTLSName(dsn, name string) (string, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", fmt.Errorf("mysql tls: parse dsn: %w", err)
	}
	cfg.TLSConfig = name
	return cfg.FormatDSN(), nil
}
