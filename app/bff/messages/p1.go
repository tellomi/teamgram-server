package messages_helper

import (
	"context"
	"strings"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/messages/webpage"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// NewPolls serves messages.sendVote and the other poll RPCs.
func NewPolls() *service.PollsService { return &service.PollsService{} }

// NewWebPage serves messages.getWebPage and messages.getWebPagePreview.
func NewWebPage(c Config) *service.WebPageService {
	BindUsers(user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)))
	return &service.WebPageService{}
}

// BindUsers installs the public username lookup used by telegram_user cards.
func BindUsers(c user_client.UserClient) {
	if c == nil {
		return
	}
	webpage.BindUserResolver(func(ctx context.Context, username string) (webpage.UserCard, bool) {
		peer, err := c.UserResolveUsername(ctx, &user.TLUserResolveUsername{Username: username})
		if err != nil || peer == nil || peer.GetUserId() == 0 {
			return webpage.UserCard{}, false
		}
		card := webpage.UserCard{ID: peer.GetUserId(), Title: username}
		list, err := c.UserGetMutableUsers(ctx, &user.TLUserGetMutableUsers{Id: []int64{peer.GetUserId()}})
		if err != nil || list == nil {
			return card, true
		}
		for _, it := range list.GetDatas() {
			data := it.GetUser()
			if data == nil || data.GetId() != peer.GetUserId() {
				continue
			}
			name := strings.TrimSpace(strings.TrimSpace(data.GetFirstName()) + " " + strings.TrimSpace(data.GetLastName()))
			if name != "" {
				card.Title = name
			}
			if data.GetAbout() != nil {
				card.About = data.GetAbout().GetValue()
			}
			card.User = mtproto.MakeTLUser(&mtproto.User{
				Id:         data.GetId(),
				AccessHash: wrapperspb.Int64(data.GetAccessHash()),
				FirstName:  wrapperspb.String(data.GetFirstName()),
				LastName:   wrapperspb.String(data.GetLastName()),
				Username:   wrapperspb.String(data.GetUsername()),
			}).To_User()
			break
		}
		return card, true
	})
}

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
// Edits and the service messages go through messenger.msg, same as messages.editMessage.
func NewTodoLists(c Config) *service.TodoListsService {
	return &service.TodoListsService{
		Msg:     msg_client.NewMsgClient(rpcx.GetCachedRpcClient(c.MsgClient)),
		Message: message_client.NewMessageClient(rpcx.GetCachedRpcClient(c.MessageClient)),
	}
}

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
