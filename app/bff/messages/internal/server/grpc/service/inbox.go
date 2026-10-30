package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

// QuickReplyService serves shortcut quick replies.
// https://core.telegram.org/api/business#quick-reply-shortcuts
type QuickReplyService struct {
	mtproto.UnimplementedRPCBusinessQuickReplyServer
	Msg msg_client.MsgClient
}

func (s QuickReplyService) MessagesGetQuickReplies(ctx context.Context, in *mtproto.TLMessagesGetQuickReplies) (*mtproto.Messages_QuickReplies, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().QuickReplies(ctx, self, in.GetHash())
}

func (s QuickReplyService) MessagesCheckQuickReplyShortcut(ctx context.Context, in *mtproto.TLMessagesCheckQuickReplyShortcut) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().CheckQuickReplyShortcut(ctx, self, in.GetShortcut())
}

func (s QuickReplyService) MessagesEditQuickReplyShortcut(ctx context.Context, in *mtproto.TLMessagesEditQuickReplyShortcut) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().EditQuickReplyShortcut(ctx, self, in.GetShortcutId(), in.GetShortcut())
}

func (s QuickReplyService) MessagesDeleteQuickReplyShortcut(ctx context.Context, in *mtproto.TLMessagesDeleteQuickReplyShortcut) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().DeleteQuickReplyShortcut(ctx, self, in.GetShortcutId())
}

func (s QuickReplyService) MessagesReorderQuickReplies(ctx context.Context, in *mtproto.TLMessagesReorderQuickReplies) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().ReorderQuickReplies(ctx, self, in.GetOrder())
}

func (s QuickReplyService) MessagesGetQuickReplyMessages(ctx context.Context, in *mtproto.TLMessagesGetQuickReplyMessages) (*mtproto.Messages_Messages, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().QuickReplyMessages(ctx, self, in.GetShortcutId(), in.GetId(), in.GetHash())
}

func (s QuickReplyService) MessagesDeleteQuickReplyMessages(ctx context.Context, in *mtproto.TLMessagesDeleteQuickReplyMessages) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	return chatbox.Shared().DeleteQuickReplyMessages(ctx, self, in.GetShortcutId(), in.GetId())
}

func (s QuickReplyService) MessagesSendQuickReplyMessages(ctx context.Context, in *mtproto.TLMessagesSendQuickReplyMessages) (*mtproto.Updates, error) {
	self, auth := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	msgs, err := chatbox.Shared().PrepareQuickReplySend(ctx, self, peer, in.GetShortcutId(), in.GetId(), in.GetRandomId())
	if err != nil {
		return nil, err
	}
	for _, random := range in.GetRandomId() {
		if ready, ok := chatbox.Shared().SeenRandom(ctx, self, random); ok {
			return ready, nil
		}
	}
	if s.Msg == nil {
		ups := localQuickUpdates(msgs)
		chatbox.Shared().NoteSent(ctx, self, peer, ups)
		for i, random := range in.GetRandomId() {
			if i < len(msgs) {
				chatbox.Shared().KeepRandom(ctx, self, random, ups)
			}
		}
		return ups, nil
	}
	boxes := make([]*msg.OutboxMessage, 0, len(msgs))
	for i, m := range msgs {
		boxes = append(boxes, msg.MakeTLOutboxMessage(&msg.OutboxMessage{
			RandomId: in.GetRandomId()[i],
			Message:  m,
		}).To_OutboxMessage())
	}
	ups, err := s.Msg.MsgSendMessageV2(ctx, &msg.TLMsgSendMessageV2{
		UserId:    self,
		AuthKeyId: auth,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		Message:   boxes,
	})
	if err != nil {
		return nil, err
	}
	chatbox.Shared().NoteSent(ctx, self, peer, ups)
	for _, random := range in.GetRandomId() {
		chatbox.Shared().KeepRandom(ctx, self, random, ups)
	}
	return ups, nil
}

func localQuickUpdates(msgs []*mtproto.Message) *mtproto.Updates {
	ups := make([]*mtproto.Update, 0, len(msgs))
	for i, m := range msgs {
		if m.GetId() == 0 {
			m.Id = int32(i + 1)
		}
		ups = append(ups, mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Message_MESSAGE: m,
			Pts_INT32:       m.Id,
			PtsCount:        1,
		}).To_Update())
	}
	return mtproto.MakeUpdatesByUpdates(ups...)
}

// SavedTagService serves reaction tags on Saved Messages.
type SavedTagService struct {
	mtproto.UnimplementedRPCSavedMessageTagsServer
}

func (SavedTagService) MessagesGetSavedReactionTags(ctx context.Context, in *mtproto.TLMessagesGetSavedReactionTags) (*mtproto.Messages_SavedReactionTags, error) {
	self, _ := selfID(ctx)
	var peer *mtproto.PeerUtil
	if in.GetPeer() != nil {
		peer = mtproto.FromInputPeer2(self, in.GetPeer())
	}
	return chatbox.Shared().SavedReactionTags(ctx, self, peer, in.GetHash())
}

func (SavedTagService) MessagesUpdateSavedReactionTag(ctx context.Context, in *mtproto.TLMessagesUpdateSavedReactionTag) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	if err := chatbox.Shared().UpdateSavedReactionTag(ctx, self, in.GetReaction(), in.GetTitle()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (SavedTagService) MessagesGetDefaultTagReactions(ctx context.Context, in *mtproto.TLMessagesGetDefaultTagReactions) (*mtproto.Messages_Reactions, error) {
	return catalog.Must().DefaultTagReactions(in.GetHash()), nil
}

// PaidMediaService serves messages.getExtendedMedia.
type PaidMediaService struct {
	mtproto.UnimplementedRPCPaidMediaServer
}

func (PaidMediaService) MessagesGetExtendedMedia(ctx context.Context, in *mtproto.TLMessagesGetExtendedMedia) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	return chatbox.Shared().ExtendedMedia(ctx, self, peer, in.GetId())
}

// DeliveryService serves messages.reportMessagesDelivery.
type DeliveryService struct {
	mtproto.UnimplementedRPCGatewayVerificationMessagesServer
}

func (DeliveryService) MessagesReportMessagesDelivery(ctx context.Context, in *mtproto.TLMessagesReportMessagesDelivery) (*mtproto.Bool, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if err := chatbox.Shared().ReportDelivery(ctx, self, peer, in.GetId(), in.GetPush()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}
