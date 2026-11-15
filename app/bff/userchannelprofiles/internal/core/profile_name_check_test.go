package core

import (
	"strings"
	"testing"

	"google.golang.org/grpc/status"
)

func TestCheckedProfileName(t *testing.T) {
	if _, err := checkedProfileName("", true); status.Convert(err).Message() != "FIRSTNAME_INVALID" {
		t.Fatalf("empty: %v", err)
	}
	if _, err := checkedProfileName("   ", true); status.Convert(err).Message() != "FIRSTNAME_INVALID" {
		t.Fatalf("spaces: %v", err)
	}
	long := strings.Repeat("名", 65)
	if _, err := checkedProfileName(long, true); status.Convert(err).Message() != "FIRSTNAME_INVALID" {
		t.Fatalf("long first: %v", err)
	}
	if _, err := checkedProfileName(long, false); status.Convert(err).Message() != "LASTNAME_INVALID" {
		t.Fatalf("long last: %v", err)
	}
	got, err := checkedProfileName("  Ada  ", true)
	if err != nil || got != "Ada" {
		t.Fatalf("trim: %q %v", got, err)
	}
	got, err = checkedProfileName("   ", false)
	if err != nil || got != "" {
		t.Fatalf("clear last: %q %v", got, err)
	}
}
