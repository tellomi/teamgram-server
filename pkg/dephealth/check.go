// Package dephealth probes the dependencies a teamgram container needs.
//
// Each probe speaks that service's own protocol and does not write a
// business row: MySQL SELECT 1, Redis PING, a Kafka metadata refresh,
// etcd's /health endpoint, and a TCP dial of the local MTProto listener.
package dephealth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/IBM/sarama"
	mysqlcfg "github.com/go-sql-driver/mysql"
	"github.com/teamgram/teamgram-server/pkg/mysqltls"
)

const defaultTimeout = 2 * time.Second

// Config is one health pass. An empty address skips that probe so a unit
// test can exercise a single dependency. The process entrypoint fills every
// field before it calls Check.
type Config struct {
	MySQLDSN    string
	RedisAddr   string
	KafkaBroker string
	EtcdURL     string
	GatewayAddr string
	Timeout     time.Duration
}

// Check runs the configured probes in order and returns the first failure.
// Each probe gets its own timeout so one stalled dependency cannot hold the
// connection past the healthcheck budget.
func Check(ctx context.Context, c Config) error {
	timeout := c.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	steps := []struct {
		name string
		run  func(context.Context) error
	}{
		{"mysql", func(ctx context.Context) error { return checkMySQL(ctx, c.MySQLDSN) }},
		{"redis", func(ctx context.Context) error { return checkRedis(ctx, c.RedisAddr) }},
		{"kafka", func(ctx context.Context) error { return checkKafka(ctx, c.KafkaBroker) }},
		{"etcd", func(ctx context.Context) error { return checkEtcd(ctx, c.EtcdURL) }},
		{"gateway", func(ctx context.Context) error { return checkGateway(ctx, c.GatewayAddr) }},
	}
	for _, step := range steps {
		stepCtx, cancel := context.WithTimeout(ctx, timeout)
		err := step.run(stepCtx)
		cancel()
		if err != nil {
			return fmt.Errorf("%s: %w", step.name, err)
		}
	}
	return nil
}

func checkMySQL(ctx context.Context, dsn string) error {
	if strings.TrimSpace(dsn) == "" {
		return nil
	}
	db, err := mysqltls.Open(dsn)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	var one int
	if err = db.QueryRowContext(ctx, "SELECT 1").Scan(&one); err != nil {
		return err
	}
	if one != 1 {
		return fmt.Errorf("SELECT 1 returned %d", one)
	}
	return nil
}

func checkRedis(ctx context.Context, addr string) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	// RESP array of one bulk string. No AUTH: the compose Redis has no password,
	// and this probe must not carry one.
	if _, err = io.WriteString(conn, "*1\r\n$4\r\nPING\r\n"); err != nil {
		return err
	}
	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		return err
	}
	reply := string(buf[:n])
	if !strings.Contains(reply, "PONG") {
		return fmt.Errorf("redis reply %q", reply)
	}
	return nil
}

func checkKafka(ctx context.Context, broker string) error {
	if strings.TrimSpace(broker) == "" {
		return nil
	}
	cfg := sarama.NewConfig()
	cfg.ClientID = "teamgram-health"
	cfg.Metadata.Full = false
	cfg.Metadata.Retry.Max = 0
	cfg.Metadata.Timeout = 2 * time.Second
	cfg.Net.DialTimeout = 2 * time.Second
	cfg.Net.ReadTimeout = 2 * time.Second
	cfg.Net.WriteTimeout = 2 * time.Second
	cfg.Consumer.Return.Errors = false
	client, err := sarama.NewClient([]string{broker}, cfg)
	if err != nil {
		return err
	}
	defer client.Close()
	done := make(chan error, 1)
	go func() {
		done <- client.RefreshMetadata()
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-done:
		return err
	}
}

func checkEtcd(ctx context.Context, rawURL string) error {
	if strings.TrimSpace(rawURL) == "" {
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("status %d", resp.StatusCode)
	}
	text := string(body)
	// etcd v3.5 returns {"health":"true"}. A false value is a failed probe.
	if strings.Contains(text, `"health":"false"`) || strings.Contains(text, `"health":false`) {
		return errors.New("etcd health is false")
	}
	if !strings.Contains(text, `"health"`) {
		return fmt.Errorf("etcd body %q", text)
	}
	return nil
}

func checkGateway(ctx context.Context, addr string) error {
	if strings.TrimSpace(addr) == "" {
		return nil
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	return conn.Close()
}

// FormatMySQLDSN builds an app-user DSN. pass is not logged by this package.
// The account is the teamgram schema user, not the migration root account.
func FormatMySQLDSN(user, pass, addr, schema string) (string, error) {
	user = strings.TrimSpace(user)
	if user == "" {
		user = "teamgram"
	}
	addr = strings.TrimSpace(addr)
	if addr == "" {
		addr = "mysql:3306"
	}
	schema = strings.TrimSpace(schema)
	if schema == "" {
		schema = "teamgram"
	}
	cfg := mysqlcfg.NewConfig()
	cfg.User = user
	cfg.Passwd = pass
	cfg.Net = "tcp"
	cfg.Addr = addr
	cfg.DBName = schema
	cfg.Timeout = defaultTimeout
	cfg.ReadTimeout = defaultTimeout
	cfg.WriteTimeout = defaultTimeout
	cfg.ParseTime = true
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	return cfg.FormatDSN(), nil
}
