package e2e

import (
	"crypto/rand"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestUsernameThreeCharacters(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "username", "User")
	var last error
	for i := 0; i < 8; i++ {
		name := randomUsername()
		cctx, cancel := contextTimeout(20 * time.Second)
		checked, err := c.Invoke(cctx, &mtproto.TLAccountCheckUsername{
			Constructor: mtproto.CRC32_account_checkUsername,
			Username:    name,
		})
		cancel()
		if err != nil {
			if rpc, ok := asRPCError(err); ok && rpc.Message == "USERNAME_OCCUPIED" {
				continue
			}
			t.Fatal(err)
		}
		if _, ok := checked.(*mtproto.TLBoolTrue); !ok {
			t.Fatalf("account.checkUsername(%s) returned %T", name, checked)
		}
		cctx, cancel = contextTimeout(20 * time.Second)
		updated, err := c.Invoke(cctx, &mtproto.TLAccountUpdateUsername{
			Constructor: mtproto.CRC32_account_updateUsername,
			Username:    name,
		})
		cancel()
		if err != nil {
			if rpc, ok := asRPCError(err); ok && rpc.Message == "USERNAME_OCCUPIED" {
				last = err
				continue
			}
			t.Fatal(err)
		}
		user, ok := updated.(*mtproto.TLUser)
		if !ok {
			t.Fatalf("account.updateUsername returned %T", updated)
		}
		got := ""
		if user.To_User().GetUsername() != nil {
			got = user.To_User().GetUsername().GetValue()
		}
		if got != name {
			t.Fatalf("username = %q, want %q", got, name)
		}
		cctx, cancel = contextTimeout(20 * time.Second)
		again, err := c.Invoke(cctx, &mtproto.TLAccountCheckUsername{
			Constructor: mtproto.CRC32_account_checkUsername,
			Username:    name,
		})
		cancel()
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := again.(*mtproto.TLBoolTrue); !ok {
			t.Fatalf("own 3-character username %s is not free for its owner: %T", name, again)
		}
		return
	}
	t.Fatalf("could not claim a 3-character username: %v", last)
}

func randomUsername() string {
	const letters = "abcdefghijklmnopqrstuvwxyz"
	var b [2]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("q%c%c", letters[int(b[0])%len(letters)], letters[int(b[1])%len(letters)])
}
