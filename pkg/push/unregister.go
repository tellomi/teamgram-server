package push

import "context"

// UnregisterSession drops device rows for authKeyIDs.
// A nil store does nothing, so logout still succeeds when push storage is
// not configured. A single 0 means every device of userID: that is the
// unbind-all path (auth.logOut with no permanent key, account deletion,
// and an enforcement reset of every session). Other zeros are skipped.
// Deleting a row that is already gone is success.
func UnregisterSession(ctx context.Context, store Store, userID int64, authKeyIDs ...int64) error {
	if store == nil || userID == 0 || len(authKeyIDs) == 0 {
		return nil
	}
	if len(authKeyIDs) == 1 && authKeyIDs[0] == 0 {
		return store.DeleteAuthKey(ctx, userID, 0)
	}
	var first error
	seen := make(map[int64]struct{}, len(authKeyIDs))
	for _, id := range authKeyIDs {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		if err := store.DeleteAuthKey(ctx, userID, id); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// RevokedAuthKeys is the auth keys whose push registrations die when a
// reset returns returned. keep is the caller's own keys and is not revoked.
// Zeros and duplicates are dropped.
func RevokedAuthKeys(keep, returned []int64) []int64 {
	skip := make(map[int64]struct{}, len(keep))
	for _, id := range keep {
		if id != 0 {
			skip[id] = struct{}{}
		}
	}
	seen := make(map[int64]struct{}, len(returned))
	out := make([]int64, 0, len(returned))
	for _, id := range returned {
		if id == 0 {
			continue
		}
		if _, ok := skip[id]; ok {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
