package push

import (
	"context"
	"time"
)

// Device is one account.registerDevice row, keyed by auth key, user, and token type.
type Device struct {
	AuthKeyID  int64
	UserID     int64
	TokenType  int32
	Token      string
	NoMuted    bool
	AppSandbox bool
	// UpdatedAt is when this registration was last written. MySQL orders
	// ListByUser by devices.updated_at. The in-memory store and the sender
	// use this field so the newest registration is not stuck behind older ones.
	UpdatedAt time.Time
	// Secret is the raw registerDevice secret. Telegram-iOS sends the 256-byte
	// master notification key when encryption is enabled (APNs) and always for VoIP.
	Secret []byte
	// LockedPeriod is account.updateDeviceLocked. A positive value means the
	// passcode is on, so pushes for this auth key omit the message text.
	LockedPeriod int32
	// LangCode is the app language. account.registerDevice copies it from the
	// session (initConnection.lang_code). A later initConnection stored on
	// auths wins when the device is loaded.
	LangCode string
	// SystemLangCode is initConnection.system_lang_code for that auth key.
	// The iOS notification extension rewrites lang_code to "en"; the phone
	// language is what PickLang uses in that case.
	SystemLangCode string
	OtherUIDs      []int64
}

// NotificationLang is the language a banner for this device is rendered in.
func (d Device) NotificationLang() string {
	return PickLang(d.LangCode, d.SystemLangCode)
}

// Store persists push devices. Implementations must be safe for concurrent use.
type Store interface {
	Upsert(ctx context.Context, d Device) error
	// Delete removes the token for userID and otherUserIDs.
	Delete(ctx context.Context, userID int64, tokenType int32, token string, otherUserIDs []int64) error
	// DeleteAuthKey removes every token registered for userID and authKeyID.
	// authKeyID 0 removes every device of that user. Deleting nothing is success.
	// This is the server-side account.unregisterDevice for a session that was
	// logged out or reset, when the client did not unregister first.
	DeleteAuthKey(ctx context.Context, userID, authKeyID int64) error
	ListByUser(ctx context.Context, userID int64) ([]Device, error)
	// SetLockedPeriod stores account.updateDeviceLocked for every device of
	// this user and auth key. period 0 clears the passcode.
	SetLockedPeriod(ctx context.Context, userID, authKeyID int64, period int32) error
}
