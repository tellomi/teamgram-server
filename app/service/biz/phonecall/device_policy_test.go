package phonecall

import (
	"context"
	"testing"
)

func TestDeviceAllowsWhenNoDatabase(t *testing.T) {
	call, secret := DeviceAllows(context.Background(), 9)
	if !call || !secret {
		t.Fatalf("call %v secret %v", call, secret)
	}
}
