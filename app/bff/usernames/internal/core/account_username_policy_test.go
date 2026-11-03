package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/pkg/usernamepolicy"

	"google.golang.org/grpc/status"
)

func TestAccountUsernamePolicyDeniesBeforeTheTable(t *testing.T) {
	t.Setenv(usernamepolicy.EnvEnabled, "1")
	c := &UsernamesCore{ctx: context.Background(), MD: &metadata.RpcMetadata{UserId: 7}}

	_, err := c.AccountCheckUsername(&mtproto.TLAccountCheckUsername{Username: "admin"})
	if status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("check admin: %v", err)
	}
	if err := c.updateUsername(7, "", "wechat"); status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("update wechat: %v", err)
	}
	if err := c.updateUsername(7, "", "admin_01"); status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("update admin_01: %v", err)
	}
}

func TestAccountUsernamePolicyAllowsThroughToTheTable(t *testing.T) {
	t.Setenv(usernamepolicy.EnvEnabled, "1")
	c := &UsernamesCore{ctx: context.Background(), MD: &metadata.RpcMetadata{UserId: 7}}
	reached := false
	func() {
		defer func() {
			if recover() != nil {
				reached = true
			}
		}()
		_, _ = c.AccountCheckUsername(&mtproto.TLAccountCheckUsername{Username: "admin01"})
	}()
	if !reached {
		t.Fatal("admin01 should pass policy and reach the user client")
	}
	reached = false
	func() {
		defer func() {
			if recover() != nil {
				reached = true
			}
		}()
		_ = c.updateUsername(777001, "", "tellomi")
	}()
	if !reached {
		t.Fatal("official tellomi should pass policy and reach the user client")
	}
}
