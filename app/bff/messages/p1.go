package messages_helper

import (
	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/server/grpc/service"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
)

// NewPolls serves messages.sendVote and the other poll RPCs.
func NewPolls() *service.PollsService { return &service.PollsService{} }

// NewWebPage serves messages.getWebPage and messages.getWebPagePreview.
func NewWebPage() *service.WebPageService { return &service.WebPageService{} }

// NewScheduled serves the scheduled-message inbox. Sending a scheduled
// message goes through messenger.msg, same as messages.sendMessage.
func NewScheduled(c Config) *service.ScheduledService {
	return &service.ScheduledService{
		Msg: msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient)),
	}
}

// NewBotChat serves inline queries, callback buttons, the attach menu,
// mini apps, and prepared inline messages. Sends go through messenger.msg.
func NewBotChat(c Config) *service.BotChatService {
	return &service.BotChatService{
		Msg: msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient)),
	}
}

// NewQuickReply serves quick-reply shortcuts. Sending a shortcut goes
// through messenger.msg, same as messages.sendMessage.
func NewQuickReply(c Config) *service.QuickReplyService {
	return &service.QuickReplyService{
		Msg: msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient)),
	}
}

// NewSavedTags serves saved-message reaction tags.
func NewSavedTags() *service.SavedTagService { return &service.SavedTagService{} }

// NewPaidMedia serves messages.getExtendedMedia.
func NewPaidMedia() *service.PaidMediaService { return &service.PaidMediaService{} }

// NewDelivery serves messages.reportMessagesDelivery.
func NewDelivery() *service.DeliveryService { return &service.DeliveryService{} }

// NewThemes serves messages.setChatTheme and the account theme RPCs.
func NewThemes() *service.ThemesService { return &service.ThemesService{} }

// NewWallpapers serves messages.setChatWallPaper and the account wallpaper RPCs.
func NewWallpapers() *service.WallpapersService { return &service.WallpapersService{} }

// NewRingtones serves account.getSavedRingtones, account.saveRingtone, and account.uploadRingtone.
func NewRingtones() *service.RingtoneService { return &service.RingtoneService{} }

// NewConnectedBots serves the connected business-bot RPCs.
func NewConnectedBots() *service.ConnectedBotsService { return &service.ConnectedBotsService{} }

// NewShareThemes serves messages.setShareTheme and messages.getShareThemeBackdrops.
func NewShareThemes() *service.ShareThemesService { return &service.ShareThemesService{} }

// NewImportedChats serves the history-import RPCs.
func NewImportedChats() *service.ImportedChatsService { return &service.ImportedChatsService{} }

// NewTodoLists serves checklist append and completion toggles.
func NewTodoLists() *service.TodoListsService { return &service.TodoListsService{} }

// NewFactChecks serves fact-check reads and edits.
func NewFactChecks() *service.FactChecksService { return &service.FactChecksService{} }

// NewAICompose serves messages.composeMessageWithAI.
func NewAICompose() *service.AIComposeService { return &service.AIComposeService{} }

// NewGames serves messages.getEmojiGameInfo.
func NewGames() *service.GamesService { return &service.GamesService{} }

// NewStatsURL serves the Android messages.getStatsURL constructor.
func NewStatsURL() *service.StatsURLService { return &service.StatsURLService{} }

// NewSuggestedPosts serves messages.toggleSuggestedPostApproval.
func NewSuggestedPosts() *service.SuggestedPostsService {
	return &service.SuggestedPostsService{}
}
