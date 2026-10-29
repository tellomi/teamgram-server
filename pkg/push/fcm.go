package push

import (
	"context"
	"errors"
)

// ErrFCMNotConfigured is returned by the default FCM sender.
// Mainland China has no FCM; wire a sender with Service.SetFCM when one exists.
var ErrFCMNotConfigured = errors.New("fcm is not configured")

// FCMSender delivers a data payload to an FCM registration token.
// The payload is the same JSON APNs posts, including "p" when a secret is set.
type FCMSender interface {
	Send(ctx context.Context, token string, payload []byte) error
}

type fcmUnconfigured struct{}

func (fcmUnconfigured) Send(context.Context, string, []byte) error {
	return ErrFCMNotConfigured
}
