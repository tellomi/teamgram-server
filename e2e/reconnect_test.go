package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestReconnectAfterGatewayRestart(t *testing.T) {
	cfg := requireE2E(t)
	if os.Getenv("TELLOMI_E2E_RESTART") == "" {
		t.Skip("set TELLOMI_E2E_RESTART=1 to restart the teamgram container")
	}
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()

	a, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	phoneA, err := cfg.Phone("reconnect_a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = a.Login(ctx, phoneA, "ReconnectA"); err != nil {
		t.Fatal(err)
	}
	b, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	phoneB, err := cfg.Phone("reconnect_b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.Login(ctx, phoneB, "ReconnectB"); err != nil {
		t.Fatal(err)
	}
	bobID, bobHash, err := a.ImportPhone(ctx, phoneB.Number, "ReconnectB")
	if err != nil {
		t.Fatal(err)
	}
	pts, qts, date, err := a.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sess, ok := a.Session()
	if !ok || sess.Key.Value.Zero() {
		t.Fatal("server did not issue a session to restore")
	}
	a.Close()

	text := fmt.Sprintf("queued-%d", time.Now().UnixNano())
	alicePeer, err := peerFor(ctx, b, phoneA.Number)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = b.SendText(ctx, alicePeer, text, nil); err != nil {
		t.Fatal(err)
	}

	if err := restartTeamgram(ctx); err != nil {
		t.Fatal(err)
	}
	// The published port stays open while the process is stopping. Wait until
	// it drops, then until bff is registered again.
	_ = waitTCPDown(ctx, cfg.Addr)
	if err := waitTCP(ctx, cfg.Addr); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(8 * time.Second):
	}

	again, err := DialSession(ctx, cfg, ProfileIOS, sess)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	diff, err := again.Difference(ctx, pts, qts, date)
	if err != nil {
		t.Fatalf("updates.getDifference after reconnect: %v", err)
	}
	if hasText(allMessages(diff), text) {
		return
	}
	err = waitUntil(ctx, time.Second, func() (bool, error) {
		msgs, err := again.History(ctx, inputPeerUser(bobID, bobHash), 30)
		if err != nil {
			return false, err
		}
		return hasText(msgs, text), nil
	})
	if err != nil {
		t.Fatalf("queued update %q was not delivered after the gateway restarted: %v", text, err)
	}
}

func peerFor(ctx context.Context, c *Client, phone string) (*mtproto.InputPeer, error) {
	id, hash, err := c.ImportPhone(ctx, phone, "ReconnectA")
	if err != nil {
		return nil, err
	}
	return inputPeerUser(id, hash), nil
}

func restartTeamgram(ctx context.Context) error {
	files := os.Getenv("TELLOMI_E2E_COMPOSE_FILES")
	if files == "" {
		files = "docker-compose-env.yaml,docker-compose.yaml"
	}
	args := []string{"compose"}
	if project := os.Getenv("COMPOSE_PROJECT_NAME"); project != "" {
		args = append(args, "-p", project)
	}
	for _, f := range strings.Split(files, ",") {
		args = append(args, "-f", strings.TrimSpace(f))
	}
	args = append(args, "restart", "teamgram")
	cmd := exec.CommandContext(ctx, "docker", args...)
	if dir := os.Getenv("TELLOMI_E2E_ROOT"); dir != "" {
		cmd.Dir = dir
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker %s: %w\n%s", strings.Join(args, " "), err, out)
	}
	return nil
}

func waitTCPDown(ctx context.Context, addr string) error {
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		d := net.Dialer{Timeout: 300 * time.Millisecond}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err != nil {
			return nil
		}
		_ = conn.Close()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(300 * time.Millisecond):
		}
	}
	return nil
}

func waitTCP(ctx context.Context, addr string) error {
	var last error
	for {
		d := net.Dialer{Timeout: time.Second}
		conn, err := d.DialContext(ctx, "tcp", addr)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		last = err
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for %s: %w", addr, last)
		case <-time.After(time.Second):
		}
	}
}
