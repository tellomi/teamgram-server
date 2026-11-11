// Command healthd is the teamgram container healthcheck.
//
// It exits 0 only when MySQL, Redis, Kafka, and etcd answer their own
// protocols and the local MTProto listener accepts a TCP connection.
// It does not listen, and it does not use the migration root account.
package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/teamgram/teamgram-server/pkg/dephealth"
)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "-check" {
		fmt.Fprintln(os.Stderr, "usage: healthd -check")
		os.Exit(2)
	}
	cfg, err := configFromEnv()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := dephealth.Check(ctx, cfg); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
}

func configFromEnv() (dephealth.Config, error) {
	dsn := strings.TrimSpace(os.Getenv("TELLOMI_HEALTH_MYSQL_DSN"))
	if dsn == "" {
		host := strings.TrimSpace(os.Getenv("MYSQL_HOST"))
		if host == "" {
			host = "mysql"
		}
		port := strings.TrimSpace(os.Getenv("MYSQL_PORT"))
		if port == "" {
			port = "3306"
		}
		schema := strings.TrimSpace(os.Getenv("MYSQL_DATABASE"))
		user := strings.TrimSpace(os.Getenv("TELLOMI_HEALTH_MYSQL_USER"))
		var err error
		dsn, err = dephealth.FormatMySQLDSN(user, os.Getenv("TEAMGRAM_MYSQL_PASSWORD"), host+":"+port, schema)
		if err != nil {
			return dephealth.Config{}, err
		}
	}
	redisAddr := strings.TrimSpace(os.Getenv("TELLOMI_HEALTH_REDIS"))
	if redisAddr == "" {
		redisAddr = "redis:6379"
	}
	broker := strings.TrimSpace(os.Getenv("TELLOMI_HEALTH_KAFKA"))
	if broker == "" {
		broker = "kafka:9092"
	}
	etcdURL := strings.TrimSpace(os.Getenv("TELLOMI_HEALTH_ETCD"))
	if etcdURL == "" {
		etcdURL = "http://etcd:2379/health"
	}
	gateway := strings.TrimSpace(os.Getenv("TELLOMI_HEALTH_GATEWAY"))
	if gateway == "" {
		gateway = "127.0.0.1:10443"
	}
	return dephealth.Config{
		MySQLDSN:    dsn,
		RedisAddr:   redisAddr,
		KafkaBroker: broker,
		EtcdURL:     etcdURL,
		GatewayAddr: gateway,
		Timeout:     2 * time.Second,
	}, nil
}
