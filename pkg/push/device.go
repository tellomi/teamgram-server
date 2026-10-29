package push

import "context"

// Device is one account.registerDevice row, keyed by auth key, user, and token type.
type Device struct {
	AuthKeyID  int64
	UserID     int64
	TokenType  int32
	Token      string
	NoMuted    bool
	AppSandbox bool
	// Secret is the raw registerDevice secret. Telegram-iOS sends the 256-byte
	// master notification key when encryption is enabled (APNs) and always for VoIP.
	Secret    []byte
	OtherUIDs []int64
}

// Store persists push devices. Implementations must be safe for concurrent use.
type Store interface {
	Upsert(ctx context.Context, d Device) error
	// Delete removes the token for userID and otherUserIDs.
	Delete(ctx context.Context, userID int64, tokenType int32, token string, otherUserIDs []int64) error
	ListByUser(ctx context.Context, userID int64) ([]Device, error)
}
