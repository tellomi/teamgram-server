package core

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/webpage"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"

	"github.com/zeromicro/go-zero/core/contextx"
	"github.com/zeromicro/go-zero/core/threading"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const pendingWebpageTimeout = 20 * time.Second

// schedulePendingWebpage fetches an external page after sendMessage (or
// editMessage) returned a webPagePending card, then edits the stored
// message for both sides with EditHide set so the transition is not shown
// as an ordinary edit. Scheduled messages are left alone until they send.
func (c *MessagesCore) schedulePendingWebpage(peer *mtproto.PeerUtil, media *mtproto.MessageMedia, text string, entities []*mtproto.MessageEntity, sent *mtproto.Updates, scheduled bool) {
	if c == nil || scheduled || !webpage.IsPending(media) || peer == nil {
		return
	}
	id := sentMessageID(sent)
	raw := chatbox.FirstURL(text, entities)
	if id == 0 || raw == "" || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return
	}
	userId := int64(0)
	authKey := int64(0)
	if c.MD != nil {
		userId = c.MD.UserId
		authKey = c.MD.PermAuthKeyId
	}
	if userId == 0 {
		return
	}
	peerType := peer.PeerType
	if peerType == mtproto.PEER_SELF {
		peerType = mtproto.PEER_USER
	}
	peerId := peer.PeerId
	ctx := contextx.ValueOnlyFrom(c.ctx)
	threading.GoSafe(func() {
		c.resolvePendingWebpage(ctx, userId, authKey, peerType, peerId, id, raw)
	})
}

func (c *MessagesCore) resolvePendingWebpage(parent context.Context, userId, authKey int64, peerType int32, peerId int64, msgID int32, raw string) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.MessageClient == nil || c.svcCtx.MsgClient == nil {
		return
	}
	ctx, cancel := context.WithTimeout(parent, pendingWebpageTimeout)
	defer cancel()
	page, _, _, err := chatbox.WebPageContext(ctx, raw, 0)
	if err != nil {
		c.Logger.Errorf("pending webpage fetch: %v", err)
		return
	}
	if page.GetPredicateName() == mtproto.Predicate_webPagePending {
		return
	}
	boxes, err := c.svcCtx.MessageClient.MessageGetUserMessageList(ctx, &message.TLMessageGetUserMessageList{
		UserId: userId,
		IdList: []int32{msgID},
	})
	if err != nil || boxes == nil || len(boxes.GetDatas()) != 1 {
		return
	}
	dst, _ := proto.Clone(boxes.Datas[0]).(*mtproto.MessageBox)
	if dst == nil || dst.Message == nil {
		return
	}
	if !webpage.IsPending(dst.Message.GetMedia()) {
		return
	}
	out := dst.Message
	webpage.ApplyFetchedPage(out, page)
	out.EditHide = true
	out.EditDate = &wrapperspb.Int32Value{Value: int32(time.Now().Unix())}
	if _, err = c.svcCtx.MsgClient.MsgEditMessageV2(ctx, &msgpb.TLMsgEditMessageV2{
		UserId:    userId,
		AuthKeyId: authKey,
		PeerType:  peerType,
		PeerId:    peerId,
		EditType:  msgpb.EditTypeNormal,
		NewMessage: msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			Message: out,
		}).To_OutboxMessage(),
		DstMessage: dst,
	}); err != nil {
		c.Logger.Errorf("pending webpage edit: %v", err)
	}
}
