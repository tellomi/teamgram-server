package dephealth

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRedisPing(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		_, _ = io.ReadFull(conn, buf[:14])
		_, _ = io.WriteString(conn, "+PONG\r\n")
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := Check(ctx, Config{RedisAddr: ln.Addr().String()}); err != nil {
		t.Fatal(err)
	}
}

func TestRedisRejectsNonPong(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		buf := make([]byte, 64)
		_, _ = conn.Read(buf)
		_, _ = io.WriteString(conn, "-NOAUTH\r\n")
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err = Check(ctx, Config{RedisAddr: ln.Addr().String()})
	if err == nil || !strings.Contains(err.Error(), "redis") {
		t.Fatalf("err %v", err)
	}
}

func TestEtcdHealth(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"health":"true"}`)
	}))
	defer ok.Close()
	if err := Check(context.Background(), Config{EtcdURL: ok.URL + "/health"}); err != nil {
		t.Fatal(err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"health":"false"}`)
	}))
	defer bad.Close()
	if err := Check(context.Background(), Config{EtcdURL: bad.URL}); err == nil {
		t.Fatal("false health was accepted")
	}
}

func TestGatewayDial(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			conn.Close()
		}
	}()
	if err := Check(context.Background(), Config{GatewayAddr: ln.Addr().String()}); err != nil {
		t.Fatal(err)
	}
	err = Check(context.Background(), Config{GatewayAddr: "127.0.0.1:1"})
	if err == nil || !strings.Contains(err.Error(), "gateway") {
		t.Fatalf("err %v", err)
	}
}

func TestEmptyConfigSkips(t *testing.T) {
	if err := Check(context.Background(), Config{}); err != nil {
		t.Fatal(err)
	}
}

func TestFormatMySQLDSNUsesAppUser(t *testing.T) {
	dsn, err := FormatMySQLDSN("", "s3cret", "mysql:3306", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(dsn, "teamgram:s3cret@tcp(mysql:3306)/teamgram") {
		t.Fatalf("dsn %s", dsn)
	}
	if strings.Contains(dsn, "root:") {
		t.Fatalf("dsn uses root: %s", dsn)
	}
}

func TestCheckStopsOnFirstFailure(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := Check(ctx, Config{
		RedisAddr:   "127.0.0.1:1",
		GatewayAddr: "127.0.0.1:1",
		Timeout:     50 * time.Millisecond,
	})
	if err == nil || !strings.HasPrefix(err.Error(), "redis:") {
		t.Fatalf("err %v", err)
	}
	if errors.Is(err, context.DeadlineExceeded) && !strings.HasPrefix(err.Error(), "redis:") {
		t.Fatal(err)
	}
}
