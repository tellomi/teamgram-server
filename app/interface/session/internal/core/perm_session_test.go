package core

import (
	"testing"

	"google.golang.org/grpc/status"
)

func TestUnboundPermSessionIsRejected(t *testing.T) {
	_, err := (&SessionCore{}).getOrFetchMainAuthWrapper(0)
	if status.Convert(err).Message() != "AUTH_KEY_PERM_EMPTY" {
		t.Fatalf("got %v", err)
	}
}
