package main

import (
	"strings"
	"testing"
)

func TestConfigUsesAppUserNotMigrationRoot(t *testing.T) {
	t.Setenv("TELLOMI_HEALTH_MYSQL_DSN", "")
	t.Setenv("TEAMGRAM_MYSQL_PASSWORD", "app-secret")
	t.Setenv("MYSQL_HOST", "mysql")
	t.Setenv("MYSQL_PORT", "3306")
	t.Setenv("MYSQL_DATABASE", "teamgram")
	t.Setenv("MYSQL_USER", "root")
	t.Setenv("MYSQL_PASSWORD", "root-secret")
	t.Setenv("TELLOMI_HEALTH_MYSQL_USER", "")
	cfg, err := configFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(cfg.MySQLDSN, "teamgram:app-secret@tcp(mysql:3306)/teamgram") {
		t.Fatalf("dsn %s", cfg.MySQLDSN)
	}
	if strings.Contains(cfg.MySQLDSN, "root") {
		t.Fatalf("health dsn uses the migration account: %s", cfg.MySQLDSN)
	}
	if cfg.RedisAddr != "redis:6379" || cfg.KafkaBroker != "kafka:9092" {
		t.Fatalf("defaults %+v", cfg)
	}
	if cfg.EtcdURL != "http://etcd:2379/health" || cfg.GatewayAddr != "127.0.0.1:10443" {
		t.Fatalf("defaults %+v", cfg)
	}
}

func TestConfigHonorsExplicitDSN(t *testing.T) {
	t.Setenv("TELLOMI_HEALTH_MYSQL_DSN", "teamgram:x@tcp(db:3306)/teamgram")
	t.Setenv("MYSQL_USER", "root")
	cfg, err := configFromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MySQLDSN != "teamgram:x@tcp(db:3306)/teamgram" {
		t.Fatalf("dsn %s", cfg.MySQLDSN)
	}
}
