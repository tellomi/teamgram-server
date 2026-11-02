package push

import (
	"context"
	"errors"
)

// ErrFCMNotConfigured is returned by the default FCM sender.
// Mainland China has no FCM; wire a sender with Service.SetFCM when one exists.
var ErrFCMNotConfigured = errors.New("fcm is not configured")

// FCMSender delivers a data payload to an FCM registration token.
// The JSON object's values are strings: loc_key, loc_args (a JSON array),
// custom (a JSON object), body (localized fallback), and p (the sealed
// loc_key object). tellomi/Telegram reads only p.
type FCMSender interface {
	Send(ctx context.Context, token string, payload []byte) error
}

type fcmUnconfigured struct{}

func (fcmUnconfigured) Send(context.Context, string, []byte) error {
	return ErrFCMNotConfigured
}
