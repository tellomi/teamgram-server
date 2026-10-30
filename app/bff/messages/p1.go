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
