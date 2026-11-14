package mtproto

import "context"

// PrivacyChatMember is installed by pkg/privacymember. It reports whether
// userID is a member of any chat in chats. known is false when membership
// cannot be checked. Nil means membership is unknown.
var PrivacyChatMember func(ctx context.Context, userID int64, chats []int64) (member bool, known bool)

// LookupChatMember asks PrivacyChatMember. A request cache filled by
// PreparePrivacyRender is answered without calling the hook. A nil hook is
// unknown membership.
func LookupChatMember(ctx context.Context, userID int64, chats []int64) (bool, bool) {
	if userID == 0 || len(chats) == 0 {
		return false, false
	}
	if member, ok := Membership(ctx, userID, chats); ok {
		return member, true
	}
	if PrivacyChatMember == nil {
		return false, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	return PrivacyChatMember(ctx, userID, chats)
}
