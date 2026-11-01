package accountassets

import "github.com/teamgram/proto/mtproto"

// EmojiList implements account.getChannelRestrictedStatusEmojis.
// No status emoji is banned, so the list is empty.
// Profile and group photo suggestions come from the sticker catalog.
// https://core.telegram.org/method/account.getChannelRestrictedStatusEmojis
func EmojiList(hash int64) *mtproto.EmojiList {
	if notModified(hash, EmptyHash) {
		return mtproto.MakeTLEmojiListNotModified(&mtproto.EmojiList{}).To_EmojiList()
	}
	return mtproto.MakeTLEmojiList(&mtproto.EmojiList{
		Hash:       EmptyHash,
		DocumentId: []int64{},
	}).To_EmojiList()
}

// CollectibleStatuses implements account.getCollectibleEmojiStatuses.
// No collectible gifts are owned or hosted here.
// https://core.telegram.org/method/account.getCollectibleEmojiStatuses
func CollectibleStatuses(hash int64) *mtproto.Account_EmojiStatuses {
	if notModified(hash, EmptyHash) {
		return mtproto.MakeTLAccountEmojiStatusesNotModified(&mtproto.Account_EmojiStatuses{}).To_Account_EmojiStatuses()
	}
	return mtproto.MakeTLAccountEmojiStatuses(&mtproto.Account_EmojiStatuses{
		Hash:     EmptyHash,
		Statuses: []*mtproto.EmojiStatus{},
	}).To_Account_EmojiStatuses()
}
