package messages_helper

import (
	"context"
	"strings"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/server/grpc/service"
	"github.com/teamgram/teamgram-server/app/bff/messages/webpage"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	media_client "github.com/teamgram/teamgram-server/app/service/media/client"
	mediapb "github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/teamgram/teamgram-server/pkg/bootstrap"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// NewPolls serves messages.sendVote and the other poll RPCs.
func NewPolls(c Config) *service.PollsService {
	svc := &service.PollsService{
		Chats:    chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
		Messages: message_client.NewMessageClient(rpcx.GetCachedRpcClient(c.MessageClient)),
	}
	if c.SyncClient != nil && len(c.SyncClient.Brokers) > 0 {
		svc.Push = sync_client.NewSyncMqClient(bootstrap.MustKafkaProducer(c.SyncClient))
	}
	return svc
}

// NewWebPage serves messages.getWebPage and messages.getWebPagePreview.
func NewWebPage(c Config) *service.WebPageService {
	BindPreview(
		user_client.NewUserClient(rpcx.GetCachedRpcClient(c.UserClient)),
		media_client.NewMediaClient(rpcx.GetCachedRpcClient(c.MediaClient)),
		chat_client.NewChatClient(rpcx.GetCachedRpcClient(c.ChatClient)),
	)
	return &service.WebPageService{}
}

// BindPreview installs username, photo, and basic-group lookups for link cards.
func BindPreview(users user_client.UserClient, media media_client.MediaClient, chats chat_client.ChatClient) {
	BindUsers(users)
	if media != nil {
		webpage.BindPhotoLoader(func(ctx context.Context, photoID int64) *mtproto.Photo {
			photo, err := media.MediaGetPhoto(ctx, &mediapb.TLMediaGetPhoto{PhotoId: photoID})
			if err != nil || photo == nil || photo.GetId() == 0 || photo.GetPredicateName() == mtproto.Predicate_photoEmpty {
				return nil
			}
			return photo
		})
	}
	if chats != nil {
		webpage.BindGroupResolver(func(ctx context.Context, hash string) (webpage.GroupCard, bool) {
			ext, err := chats.ChatCheckChatInvite(ctx, &chat.TLChatCheckChatInvite{
				SelfId: viewerFrom(ctx),
				Hash:   hash,
			})
			if err != nil || ext == nil {
				return webpage.GroupCard{}, false
			}
			return groupCardFrom(ext), true
		})
	}
}

func viewerFrom(ctx context.Context) int64 {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0
	}
	return md.UserId
}

func groupCardFrom(ext *chat.ChatInviteExt) webpage.GroupCard {
	if ext == nil {
		return webpage.GroupCard{}
	}
	if ext.GetChat() != nil {
		ch := ext.GetChat().ToUnsafeChat(viewerZero)
		card := webpage.GroupCard{
			Title:   ext.GetChat().Title(),
			About:   ext.GetChat().About(),
			Members: ext.GetChat().ParticipantsCount(),
			Photo:   ext.GetChat().Photo(),
			Chat:    ch,
		}
		if card.Title == "" && ch != nil {
			card.Title = ch.GetTitle()
		}
		return card
	}
	about := ""
	if ext.GetAbout() != nil {
		about = ext.GetAbout().GetValue()
	}
	return webpage.GroupCard{
		Title:   ext.GetTitle(),
		About:   about,
		Members: ext.GetParticipantsCount(),
		Photo:   ext.GetPhoto(),
	}
}

// viewerZero is only used when ToUnsafeChat needs a self id for flags that
// do not change the title or the photo. The real viewer is applied by the
// chat service before it returns the invite.
const viewerZero = 0

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
		card := webpage.UserCard{ID: peer.GetUserId(), Title: username, Username: username}
		viewer := viewerFrom(ctx)
		list, err := c.UserGetMutableUsers(ctx, &user.TLUserGetMutableUsers{
			Id: []int64{peer.GetUserId()},
			To: []int64{viewer},
		})
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
			if data.GetUsername() != "" {
				card.Username = data.GetUsername()
			}
			card.Bot = it.IsBot()
			if photo := it.Photo(); photo != nil && it.CheckPrivacy(mtproto.PROFILE_PHOTO, viewer) {
				card.Photo = photo
			}
			tl := mtproto.MakeTLUser(&mtproto.User{
				Bot:        card.Bot,
				Id:         data.GetId(),
				AccessHash: wrapperspb.Int64(data.GetAccessHash()),
				FirstName:  wrapperspb.String(data.GetFirstName()),
				LastName:   wrapperspb.String(data.GetLastName()),
				Username:   wrapperspb.String(card.Username),
			}).To_User()
			if card.Photo != nil {
				tl.Photo = mtproto.MakeUserProfilePhotoByPhoto(card.Photo)
			}
			card.User = tl
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
