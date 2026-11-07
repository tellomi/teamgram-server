package core

import (
	"context"
	"testing"

	"github.com/teamgram/teamgram-server/pkg/usernamepolicy"
	"google.golang.org/grpc/status"
)

func TestSignupDisplayNameUsesLexicon(t *testing.T) {
	ctx := context.Background()
	t.Setenv(usernamepolicy.EnvEnabled, "")
	if err := rejectSignupName(ctx, "admin", "tellomi"); err != nil {
		t.Fatalf("policy off: %v", err)
	}

	t.Setenv(usernamepolicy.EnvEnabled, "1")
	if err := rejectSignupName(ctx, "小明", ""); err != nil {
		t.Fatalf("ordinary name: %v", err)
	}
	if err := rejectSignupName(ctx, "Ada", "Lee"); err != nil {
		t.Fatalf("ordinary latin: %v", err)
	}
	if status.Convert(rejectSignupName(ctx, "admin", "Lee")).Message() != "FIRSTNAME_INVALID" {
		t.Fatalf("first: %v", rejectSignupName(ctx, "admin", "Lee"))
	}
	if status.Convert(rejectSignupName(ctx, "小明", "tellomi")).Message() != "LASTNAME_INVALID" {
		t.Fatalf("last: %v", rejectSignupName(ctx, "小明", "tellomi"))
	}
	if status.Convert(rejectSignupName(ctx, "Tel1omi", "")).Message() != "FIRSTNAME_INVALID" {
		t.Fatal("variant was allowed")
	}
}
