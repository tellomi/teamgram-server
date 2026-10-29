package bootstrap

import (
	"net"
	"strings"
	"testing"
	"time"
)

func TestMySQLAddr(t *testing.T) {
	got := MySQLAddr("teamgram:teamgram@tcp(mysql:3306)/teamgram?charset=utf8mb4")
	if got != "mysql:3306" {
		t.Fatalf("addr %q", got)
	}
	if MySQLAddr("not-a-dsn") != "" {
		t.Fatal("expected empty")
	}
}

func TestWaitRetriesUntilAccept(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	addr := ln.Addr().String()
	// First dial fails, then the real listener accepts.
	calls := 0
	restore := SetRetryForTest(5, func(time.Duration) {}, func(network, address string, timeout time.Duration) (net.Conn, error) {
		calls++
		if calls == 1 {
			return nil, &net.OpError{Op: "dial", Net: network, Err: errClosed}
		}
		return net.DialTimeout(network, address, timeout)
	})
	defer restore()
	Wait("test", Deps{Etcd: []string{addr}})
	if calls < 2 {
		t.Fatalf("calls = %d", calls)
	}
}

func TestKafkaProducerGivesUp(t *testing.T) {
	restore := SetRetryForTest(2, func(time.Duration) {}, nil)
	defer restore()
	_, err := retryValue("kafka producer 127.0.0.1:1", func() (*struct{}, error) {
		return nil, errClosed
	})
	if err == nil || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("err = %v", err)
	}
}

var errClosed = net.ErrClosed
