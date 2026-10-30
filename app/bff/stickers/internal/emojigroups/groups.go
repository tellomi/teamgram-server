// Package emojigroups answers messages.getEmoji*Groups.
//
// The catalog has no emoji groups. Clients that already stored the empty-list
// hash receive messages.emojiGroupsNotModified.
//
// https://core.telegram.org/method/messages.getEmojiGroups
package emojigroups

import "github.com/teamgram/proto/mtproto"

// Hash is the hash of the empty group list.
const Hash int32 = 1

// List returns the emoji group page for hash.
func List(hash int32) *mtproto.Messages_EmojiGroups {
	if hash == Hash {
		return mtproto.MakeTLMessagesEmojiGroupsNotModified(&mtproto.Messages_EmojiGroups{}).To_Messages_EmojiGroups()
	}
	return mtproto.MakeTLMessagesEmojiGroups(&mtproto.Messages_EmojiGroups{
		Hash:   Hash,
		Groups: []*mtproto.EmojiGroup{},
	}).To_Messages_EmojiGroups()
}
