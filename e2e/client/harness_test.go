package e2e

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func requireE2E(t *testing.T) Config {
	t.Helper()
	if os.Getenv("TELLOMI_E2E") == "" {
		t.Skip("set TELLOMI_E2E=1 to run against a Teamgram stack")
	}
	cfg, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

var (
	sharedOnce sync.Once
	sharedCfg  Config
	sharedErr  error
	alice      *Client
	bob        *Client
	aliceSelf  Self
	bobSelf    Self
	aliceSeen  Self
)

func sharedPair(t *testing.T) (a, b *Client, aSelf, bSelf Self) {
	t.Helper()
	cfg := requireE2E(t)
	sharedOnce.Do(func() {
		sharedCfg = cfg
		sharedErr = setupPair(cfg)
	})
	if sharedErr != nil {
		t.Fatal(sharedErr)
	}
	if !clientAlive(alice) || !clientAlive(bob) {
		if alice != nil {
			alice.Close()
		}
		if bob != nil {
			bob.Close()
		}
		if err := setupPair(sharedCfg); err != nil {
			t.Fatal(err)
		}
	}
	return alice, bob, aliceSelf, bobSelf
}

func clientAlive(c *Client) bool {
	if c == nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.Invoke(ctx, &mtproto.TLHelpGetNearestDc{Constructor: mtproto.CRC32_help_getNearestDc})
	return err == nil
}

func setupPair(cfg Config) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var err error
	alice, err = Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		return fmt.Errorf("dial alice: %w", err)
	}
	phone, err := cfg.Phone("alice")
	if err != nil {
		return err
	}
	aliceSelf, err = alice.Login(ctx, phone, "Alice")
	if err != nil {
		return err
	}
	bob, err = Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		return fmt.Errorf("dial bob: %w", err)
	}
	phone, err = cfg.Phone("bob")
	if err != nil {
		return err
	}
	bobSelf, err = bob.Login(ctx, phone, "Bob")
	if err != nil {
		return err
	}
	bobID, bobHash, err := alice.ImportPhone(ctx, phone.Number, "Bob")
	if err != nil {
		return fmt.Errorf("import bob: %w", err)
	}
	bobSelf.ID = bobID
	bobSelf.AccessHash = bobHash
	alicePhone, err := cfg.Phone("alice")
	if err != nil {
		return err
	}
	aliceID, aliceHash, err := bob.ImportPhone(ctx, alicePhone.Number, "Alice")
	if err != nil {
		return fmt.Errorf("import alice: %w", err)
	}
	aliceSeen = Self{ID: aliceID, AccessHash: aliceHash}
	return nil
}

func loginNamed(t *testing.T, cfg Config, profile Profile, name, first string) (*Client, Self) {
	t.Helper()
	phone, err := cfg.Phone(name)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, err := Dial(ctx, cfg, profile)
	if err != nil {
		t.Fatalf("dial %s: %v", name, err)
	}
	t.Cleanup(c.Close)
	self, err := c.Login(ctx, phone, first)
	if err != nil {
		t.Fatal(err)
	}
	return c, self
}

func invokeOK(t *testing.T, c *Client, req mtproto.TLObject) mtproto.TLObject {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	obj, err := c.Invoke(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if obj == nil {
		t.Fatal("empty result")
	}
	return obj
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 20*time.Second)
}

func contextTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}
