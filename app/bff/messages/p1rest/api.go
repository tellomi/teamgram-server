package p1rest

import "github.com/teamgram/proto/mtproto"

// SetChatTheme implements messages.setChatTheme.
// https://core.telegram.org/method/messages.setChatTheme
func SetChatTheme(self int64, in *mtproto.TLMessagesSetChatTheme) (*mtproto.Updates, error) {
	return shared().setChatTheme(self, in)
}

// SetChatWallPaper implements messages.setChatWallPaper.
// https://core.telegram.org/method/messages.setChatWallPaper
func SetChatWallPaper(self int64, in *mtproto.TLMessagesSetChatWallPaper) (*mtproto.Updates, error) {
	return shared().setWallpaper(self, in)
}

// SetShareTheme implements messages.setShareTheme.
func SetShareTheme(self int64, peer *mtproto.InputPeer, theme *mtproto.ShareTheme) (*mtproto.Bool, error) {
	return shared().setShareTheme(self, peer, theme)
}

// ShareThemeBackdrops implements messages.getShareThemeBackdrops.
func ShareThemeBackdrops(hash int32) *mtproto.ShareThemeBackdrops {
	return shareBackdrops(hash)
}

// CheckHistoryImport implements messages.checkHistoryImport.
// https://core.telegram.org/method/messages.checkHistoryImport
func CheckHistoryImport(head string) (*mtproto.Messages_HistoryImportParsed, error) {
	return checkHistory(head)
}

// CheckHistoryImportPeer implements messages.checkHistoryImportPeer.
// https://core.telegram.org/method/messages.checkHistoryImportPeer
func CheckHistoryImportPeer(self int64, peer *mtproto.InputPeer) (*mtproto.Messages_CheckedHistoryImportPeer, error) {
	return shared().checkPeer(self, peer)
}

// InitHistoryImport implements messages.initHistoryImport.
// https://core.telegram.org/method/messages.initHistoryImport
func InitHistoryImport(self int64, in *mtproto.TLMessagesInitHistoryImport) (*mtproto.Messages_HistoryImport, error) {
	return shared().initImport(self, in)
}

// UploadImportedMedia implements messages.uploadImportedMedia.
// https://core.telegram.org/method/messages.uploadImportedMedia
func UploadImportedMedia(self int64, in *mtproto.TLMessagesUploadImportedMedia) (*mtproto.MessageMedia, error) {
	return shared().uploadImported(self, in)
}

// StartHistoryImport implements messages.startHistoryImport.
// https://core.telegram.org/method/messages.startHistoryImport
func StartHistoryImport(self int64, in *mtproto.TLMessagesStartHistoryImport) (*mtproto.Bool, error) {
	return shared().startImport(self, in)
}

// AppendTodoList implements messages.appendTodoList.
// https://core.telegram.org/method/messages.appendTodoList
func AppendTodoList(self int64, in *mtproto.TLMessagesAppendTodoList) (*mtproto.Updates, error) {
	return shared().appendTodo(self, in)
}

// ToggleTodoCompleted implements messages.toggleTodoCompleted.
// https://core.telegram.org/method/messages.toggleTodoCompleted
func ToggleTodoCompleted(self int64, in *mtproto.TLMessagesToggleTodoCompleted) (*mtproto.Updates, error) {
	return shared().toggleTodo(self, in)
}

// GetFactCheck implements messages.getFactCheck.
// https://core.telegram.org/method/messages.getFactCheck
func GetFactCheck(self int64, peer *mtproto.InputPeer, ids []int32) (*mtproto.Vector_FactCheck, error) {
	return factChecks(self, peer, ids)
}

// EditFactCheck implements messages.editFactCheck.
// https://core.telegram.org/method/messages.editFactCheck
func EditFactCheck(self int64, peer *mtproto.InputPeer, msgID int32, text *mtproto.TextWithEntities) (*mtproto.Updates, error) {
	return editFact(self, peer, msgID, text)
}

// DeleteFactCheck implements messages.deleteFactCheck.
// https://core.telegram.org/method/messages.deleteFactCheck
func DeleteFactCheck(self int64, peer *mtproto.InputPeer, msgID int32) (*mtproto.Updates, error) {
	return deleteFact(self, peer, msgID)
}

// ComposeMessageWithAI implements messages.composeMessageWithAI.
// https://core.telegram.org/method/messages.composeMessageWithAI
func ComposeMessageWithAI(in *mtproto.TLMessagesComposeMessageWithAI) (*mtproto.Messages_ComposedMessageWithAI, error) {
	return composeAI(in)
}

// EmojiGameInfo implements messages.getEmojiGameInfo.
// https://core.telegram.org/method/messages.getEmojiGameInfo
func EmojiGameInfo() *mtproto.Messages_EmojiGameInfo {
	return emojiGame()
}

// StatsURL implements the Android messages.getStatsURL body.
// https://core.telegram.org/method/messages.getStatsURL
func StatsURL(self int64, in *mtproto.TLMessagesGetStatsURL) (*mtproto.StatsURL, error) {
	return statsURL(self, in)
}

// ToggleSuggestedPostApproval implements messages.toggleSuggestedPostApproval.
// https://core.telegram.org/method/messages.toggleSuggestedPostApproval
func ToggleSuggestedPostApproval(self int64, in *mtproto.TLMessagesToggleSuggestedPostApproval) (*mtproto.Updates, error) {
	return shared().suggest(self, in)
}
