package push

import (
	"context"
	"errors"
)

// ErrHuaweiNotConfigured is returned until a Huawei Push Kit sender is installed.
// The Android client registers token_type 13 (PUSH_TYPE_HUAWEI). There is no
// Xiaomi, OPPO, vivo, or JPush token in the Telegram API, and this server does
// not send those. Mainland China builds that cannot reach FCM use this token.
var ErrHuaweiNotConfigured = errors.New("huawei push is not configured")

// HuaweiSender delivers the encrypted notification string to one HMS token.
// The string is the same "p" value FCM puts in its data map. The Tellomi
// Huawei listener base64-decodes that string directly.
type HuaweiSender interface {
	Send(ctx context.Context, token string, data string) error
}

type huaweiUnconfigured struct{}

func (huaweiUnconfigured) Send(context.Context, string, string) error {
	return ErrHuaweiNotConfigured
}
