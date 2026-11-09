package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"
	"github.com/zeromicro/go-zero/core/contextx"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/core/threading"
)

// noteTellomi replies as @tellomi after a message is stored.
// Private chats to 777001 keep the existing path. Basic groups and
// supergroups reply only to a mention of @tellomi or a reply to 777001.
// Broadcast channels, bots, and 777001 itself are ignored. Scheduled sends
// are not passed here.
func (c *MessagesCore) noteTellomi(from int64, peer *mtproto.PeerUtil, text string, entities []*mtproto.MessageEntity, random int64, replyTo int32, sent *mtproto.Updates) {
	if peer == nil || from == 0 || from == tellomiai.AccountID {
		return
	}
	if tellomiPrivate(from, peer) {
		maybeTellomi(c.ctx, from, peer, text, random, replyTo)
		return
	}
	if c.skipGroup(from, peer) || !tellomiai.Enabled() {
		return
	}
	sentID := sentMessageID(sent)
	if sentID == 0 {
		return
	}
	copied := append([]*mtproto.MessageEntity(nil), entities...)
	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	ctx = contextx.ValueOnlyFrom(ctx)
	threading.GoSafe(func() {
		c.answerTellomiGroup(ctx, from, peer.PeerType, peer.PeerId, text, copied, random, replyTo, sentID)
	})
}

func (c *MessagesCore) skipGroup(from int64, peer *mtproto.PeerUtil) bool {
	isBot := c != nil && c.MD != nil && c.MD.IsBot
	return from == 0 || from == tellomiai.AccountID || isBot || !tellomiGroupPeer(peer)
}

func tellomiGroupPeer(peer *mtproto.PeerUtil) bool {
	if peer == nil {
		return false
	}
	return peer.PeerType == mtproto.PEER_CHAT || peer.PeerType == mtproto.PEER_CHANNEL
}

func sentMessageID(up *mtproto.Updates) int32 {
	if up == nil {
		return 0
	}
	for _, u := range up.GetUpdates() {
		if m := u.GetMessage_MESSAGE(); m != nil && m.GetId() != 0 {
			return m.GetId()
		}
	}
	return 0
}

// answerTellomiGroup posts one reply into the same chat. The model sees only
// the triggering text (mention removed) and, when this message is a reply,
// that one replied-to message. It does not read the rest of the chat.
func (c *MessagesCore) answerTellomiGroup(ctx context.Context, from int64, peerType int32, peerID int64, text string, entities []*mtproto.MessageEntity, random int64, replyTo, sentID int32) {
	if from == 0 || from == tellomiai.AccountID || peerID == 0 || sentID == 0 {
		return
	}
	ents, hasEntities := mentionsFrom(entities)
	question, mentioned := tellomiai.StripMention(text, ents, !hasEntities)
	replied, replyToAI, replyID, ok := c.groupContext(ctx, from, peerType, peerID, replyTo, sentID)
	if !ok || (!mentioned && !replyToAI) {
		return
	}
	if !c.tellomiCanPost(ctx, peerType, peerID) {
		return
	}
	prompt := tellomiai.GroupPrompt(question, replied)
	if prompt == "" {
		return
	}
	tellomiai.ObserveGroup(ctx, tellomiai.GroupAsk{
		From: from, PeerType: peerType, PeerID: peerID,
		ReplyTo: replyID, Text: prompt, Random: random,
	})
}

// groupContext loads at most the one message this send replies to, and the
// id of the triggering message in 777001's own box. It does not list history.
func (c *MessagesCore) groupContext(ctx context.Context, from int64, peerType int32, peerID int64, replyTo, sentID int32) (replied string, replyToAI bool, replyID int32, ok bool) {
	if sentID == 0 {
		return "", false, 0, false
	}
	if replyTo != 0 {
		sender, body, found := c.oneReplied(ctx, from, peerType, peerID, replyTo)
		if found {
			replied = body
			replyToAI = sender == tellomiai.AccountID
		}
	}
	if peerType == mtproto.PEER_CHANNEL {
		return replied, replyToAI, sentID, true
	}
	if peerType != mtproto.PEER_CHAT {
		return "", false, 0, false
	}
	mapped, found := c.tellomiBoxID(ctx, from, sentID)
	if !found {
		return replied, replyToAI, 0, false
	}
	return replied, replyToAI, mapped, true
}

func (c *MessagesCore) oneReplied(ctx context.Context, from int64, peerType int32, peerID int64, replyTo int32) (int64, string, bool) {
	if replyTo == 0 {
		return 0, "", false
	}
	if peerType == mtproto.PEER_CHANNEL {
		sender, msg, err := channel.Shared().LoadMessageAuthor(ctx, peerID, replyTo)
		if err != nil || msg == nil {
			return 0, "", false
		}
		if sender == 0 && msg.GetFromId() != nil {
			sender = msg.GetFromId().GetUserId()
		}
		return sender, msg.GetMessage(), true
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return 0, "", false
	}
	box, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessage(ctx, &message.TLMessageGetUserMessage{
		UserId: from,
		Id:     replyTo,
	})
	if err != nil || box == nil || box.GetMessage() == nil {
		return 0, "", false
	}
	sender := box.GetSenderUserId()
	if sender == 0 && box.GetMessage().GetFromId() != nil {
		sender = box.GetMessage().GetFromId().GetUserId()
	}
	return sender, box.GetMessage().GetMessage(), true
}

func (c *MessagesCore) tellomiBoxID(ctx context.Context, from int64, sentID int32) (int32, bool) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil || sentID == 0 {
		return 0, false
	}
	box, err := c.svcCtx.Dao.MessageClient.MessageGetPeerUserMessage(ctx, &message.TLMessageGetPeerUserMessage{
		UserId:     from,
		PeerUserId: tellomiai.AccountID,
		MsgId:      sentID,
	})
	if err != nil || box == nil || box.GetMessageId() == 0 {
		return 0, false
	}
	return box.GetMessageId(), true
}

// tellomiCanPost reports whether 777001 can post in this chat.
// Basic groups need a current participant: sendChatOutgoingMessageV2 returns
// CHAT_WRITE_FORBIDDEN when the sender has no participant row. Supergroups use
// channel.CanPost, the same check as msg.sendMessageV2. Broadcast channels
// are false.
func (c *MessagesCore) tellomiCanPost(ctx context.Context, peerType int32, peerID int64) bool {
	switch peerType {
	case mtproto.PEER_CHAT:
		return c.basicCanPost(ctx, peerID)
	case mtproto.PEER_CHANNEL:
		return channel.Shared().CanPost(ctx, peerID, tellomiai.AccountID) == nil
	default:
		return false
	}
}

func (c *MessagesCore) basicCanPost(ctx context.Context, chatID int64) bool {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.ChatClient == nil || c.svcCtx.Dao.ChatClient.Client() == nil {
		return false
	}
	chat, err := c.svcCtx.Dao.ChatClient.Client().ChatGetMutableChat(ctx, &chatpb.TLChatGetMutableChat{ChatId: chatID})
	if err != nil {
		logx.WithContext(ctx).Errorf("tellomi ai: basic group lookup chat=%d", chatID)
		return false
	}
	return basicMember(chat, tellomiai.AccountID)
}

func basicMember(chat *mtproto.MutableChat, userID int64) bool {
	if chat == nil || chat.Deactivated() {
		return false
	}
	p, ok := chat.GetImmutableChatParticipant(userID)
	return ok && p != nil && p.IsChatMemberStateNormal()
}

func mentionsFrom(entities []*mtproto.MessageEntity) ([]tellomiai.Mention, bool) {
	if len(entities) == 0 {
		return nil, false
	}
	var out []tellomiai.Mention
	for _, e := range entities {
		if e == nil {
			continue
		}
		m := tellomiai.Mention{Offset: int(e.GetOffset()), Length: int(e.GetLength())}
		switch e.GetPredicateName() {
		case mtproto.Predicate_messageEntityMention:
			m.Kind = tellomiai.MentionUsername
			m.UserID = e.GetUserId_INT64()
		case mtproto.Predicate_messageEntityMentionName:
			m.Kind = tellomiai.MentionName
			m.UserID = e.GetUserId_INT64()
		case mtproto.Predicate_inputMessageEntityMentionName:
			m.Kind = tellomiai.MentionName
			m.UserID = e.GetUserId_INPUTUSER().GetUserId()
		default:
			continue
		}
		out = append(out, m)
	}
	return out, true
}
