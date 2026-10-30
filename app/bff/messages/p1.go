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
