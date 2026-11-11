package session

// SetPushUpdatesUser records the user an updates push is for.
func SetPushUpdatesUser(msg *TLSessionPushUpdatesData, userID int64) {
	if msg == nil || userID <= 0 {
		return
	}
	msg.UserId = userID
}

// PushUpdatesUser returns the user stamped by SetPushUpdatesUser.
func PushUpdatesUser(msg *TLSessionPushUpdatesData) int64 {
	if msg == nil {
		return 0
	}
	return msg.GetUserId()
}

// SetPushSessionUpdatesUser records the user a single-session push is for.
func SetPushSessionUpdatesUser(msg *TLSessionPushSessionUpdatesData, userID int64) {
	if msg == nil || userID <= 0 {
		return
	}
	msg.UserId = userID
}

// PushSessionUpdatesUser returns the user stamped by SetPushSessionUpdatesUser.
func PushSessionUpdatesUser(msg *TLSessionPushSessionUpdatesData) int64 {
	if msg == nil {
		return 0
	}
	return msg.GetUserId()
}
