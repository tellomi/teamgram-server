package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/deeplinks/internal/core"
	"github.com/teamgram/teamgram-server/app/bff/deeplinks/internal/svc"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

type Service struct {
	mtproto.UnimplementedRPCDeepLinksServer
	svcCtx *svc.ServiceContext
}

func New(ctx *svc.ServiceContext) *Service {
	return &Service{svcCtx: ctx}
}

func (s *Service) HelpGetDeepLinkInfo(ctx context.Context, request *mtproto.TLHelpGetDeepLinkInfo) (*mtproto.Help_DeepLinkInfo, error) {
	return core.New(ctx, s.svcCtx).HelpGetDeepLinkInfo(request)
}

func (s *Service) HelpGetRecentMeUrls(ctx context.Context, request *mtproto.TLHelpGetRecentMeUrls) (*mtproto.Help_RecentMeUrls, error) {
	return core.New(ctx, s.svcCtx).HelpGetRecentMeUrls(request)
}

// MessagesStartBot sends /start (or /start <param>) into the peer.
// https://core.telegram.org/method/messages.startBot
func (s *Service) MessagesStartBot(ctx context.Context, in *mtproto.TLMessagesStartBot) (*mtproto.Updates, error) {
	md := metadata.RpcMetadataFromIncoming(ctx)
	var self, auth int64
	if md != nil {
		self, auth = md.GetUserId(), md.GetPermAuthKeyId()
	}
	if ups, ok := chatbox.Shared().SeenRandom(ctx, self, in.GetRandomId()); ok {
		return ups, nil
	}
	req, err := chatbox.Shared().StartBot(ctx, self, in.GetBot(), in.GetPeer(), in.GetRandomId(), in.GetStartParam())
	if err != nil {
		return nil, err
	}
	var ups *mtproto.Updates
	if s.svcCtx.Msg == nil {
		ups = req.Fallback
	} else {
		ups, err = s.svcCtx.Msg.MsgSendMessageV2(ctx, &msg.TLMsgSendMessageV2{
			UserId:    self,
			AuthKeyId: auth,
			PeerType:  req.Peer.PeerType,
			PeerId:    req.Peer.PeerId,
			Message: []*msg.OutboxMessage{
				msg.MakeTLOutboxMessage(&msg.OutboxMessage{
					RandomId: req.RandomID,
					Message:  req.Message,
				}).To_OutboxMessage(),
			},
		})
		if err != nil {
			return nil, err
		}
	}
	chatbox.Shared().KeepRandom(ctx, self, in.GetRandomId(), ups)
	return ups, nil
}
