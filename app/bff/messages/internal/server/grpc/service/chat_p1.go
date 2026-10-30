package service

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"google.golang.org/protobuf/proto"
)

func selfID(ctx context.Context) (int64, int64) {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md == nil {
		return 0, 0
	}
	return md.GetUserId(), md.GetPermAuthKeyId()
}

// PollsService serves the messages poll RPCs.
// https://core.telegram.org/api/poll
type PollsService struct {
	mtproto.UnimplementedRPCPollsServer
}

func (PollsService) MessagesSendVote(ctx context.Context, in *mtproto.TLMessagesSendVote) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	pollID, err := ensurePollID(ctx, self, peer, in.GetMsgId())
	if err != nil {
		return nil, err
	}
	ups, err := chatbox.Shared().Vote(ctx, self, peer.PeerType, peer.PeerId, in.GetMsgId(), pollID, in.GetOptions())
	if err != nil {
		return nil, err
	}
	writePollBack(ctx, peer, in.GetMsgId(), pollID)
	return ups, nil
}

func (PollsService) MessagesGetPollResults(ctx context.Context, in *mtproto.TLMessagesGetPollResults) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	pollID, err := ensurePollID(ctx, self, peer, in.GetMsgId())
	if err != nil {
		return nil, err
	}
	return chatbox.Shared().PollResultsUpdate(ctx, self, peer.PeerType, peer.PeerId, in.GetMsgId(), pollID, in.GetPollHash())
}

func (PollsService) MessagesGetPollVotes(ctx context.Context, in *mtproto.TLMessagesGetPollVotes) (*mtproto.Messages_VotesList, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	pollID, err := ensurePollID(ctx, self, peer, in.GetId())
	if err != nil {
		return nil, err
	}
	offset := ""
	if in.GetOffset() != nil {
		offset = in.GetOffset().GetValue()
	}
	return chatbox.Shared().PollVotes(ctx, self, peer.PeerType, peer.PeerId, in.GetId(), pollID, in.GetOption(), offset, in.GetLimit())
}

func (PollsService) MessagesAddPollAnswer(ctx context.Context, in *mtproto.TLMessagesAddPollAnswer) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	pollID, err := ensurePollID(ctx, self, peer, in.GetMsgId())
	if err != nil {
		return nil, err
	}
	ups, err := chatbox.Shared().AddPollAnswer(ctx, self, peer.PeerType, peer.PeerId, in.GetMsgId(), pollID, in.GetAnswer())
	if err != nil {
		return nil, err
	}
	writePollBack(ctx, peer, in.GetMsgId(), pollID)
	return ups, nil
}

func (PollsService) MessagesDeletePollAnswer(ctx context.Context, in *mtproto.TLMessagesDeletePollAnswer) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	pollID, err := ensurePollID(ctx, self, peer, in.GetMsgId())
	if err != nil {
		return nil, err
	}
	ups, err := chatbox.Shared().DeletePollAnswer(ctx, self, peer.PeerType, peer.PeerId, in.GetMsgId(), pollID, in.GetOption())
	if err != nil {
		return nil, err
	}
	writePollBack(ctx, peer, in.GetMsgId(), pollID)
	return ups, nil
}

func (PollsService) MessagesGetUnreadPollVotes(ctx context.Context, in *mtproto.TLMessagesGetUnreadPollVotes) (*mtproto.Messages_Messages, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	msgs, err := chatbox.Shared().UnreadPollMessages(ctx, self, peer.PeerType, peer.PeerId, in.GetOffsetId(), in.GetAddOffset(), in.GetLimit(), in.GetMaxId(), in.GetMinId())
	if err != nil {
		return nil, err
	}
	var chats []*mtproto.Chat
	var pts int32
	if peer.IsChannel() {
		pts, chats = channelCard(ctx, self, peer.PeerId)
		for i, m := range msgs {
			body, err := channel.Shared().LoadMessage(ctx, peer.PeerId, m.GetId())
			if err != nil {
				continue
			}
			body.Media = m.Media
			msgs[i] = body
		}
	}
	return chatbox.PageMessages(peer, pts, msgs, chats), nil
}

func (PollsService) MessagesReadPollVotes(ctx context.Context, in *mtproto.TLMessagesReadPollVotes) (*mtproto.Messages_AffectedHistory, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if err := chatbox.Shared().ReadPollVotes(ctx, self, peer.PeerType, peer.PeerId); err != nil {
		return nil, err
	}
	var pts int32
	if peer.IsChannel() {
		pts, _ = channelCard(ctx, self, peer.PeerId)
	}
	return chatbox.AffectedHistory(pts, 0), nil
}

func ensurePollID(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgID int32) (int64, error) {
	if !peer.IsChannel() {
		return 0, nil
	}
	msg, err := channel.Shared().LoadMessage(ctx, peer.PeerId, msgID)
	if err != nil {
		return 0, err
	}
	id, err := mtproto.GetPollIdByMessage(msg.GetMedia())
	if err != nil {
		return 0, mtproto.ErrMessageIdInvalid
	}
	creator := self
	if msg.GetFromId() != nil && msg.GetFromId().GetUserId() != 0 {
		creator = msg.GetFromId().GetUserId()
	}
	if err := chatbox.Shared().EnsurePoll(ctx, creator, peer.PeerType, peer.PeerId, msgID, msg.GetMedia().GetPoll()); err != nil {
		return 0, err
	}
	return id, nil
}

func writePollBack(ctx context.Context, peer *mtproto.PeerUtil, msgID int32, pollID int64) {
	if peer == nil || !peer.IsChannel() || pollID == 0 {
		return
	}
	poll, results := chatbox.Shared().PublicResults(ctx, pollID)
	if poll == nil {
		return
	}
	msg, err := channel.Shared().LoadMessage(ctx, peer.PeerId, msgID)
	if err != nil || msg.GetMedia() == nil {
		return
	}
	msg.Media.Poll = poll
	msg.Media.Results = results
	_ = channel.Shared().SaveMessageBody(ctx, peer.PeerId, msgID, msg)
}

func channelCard(ctx context.Context, self, channelID int64) (int32, []*mtproto.Chat) {
	pts, chat, err := channel.Shared().Card(ctx, self, channelID)
	if err != nil || chat == nil {
		return 0, nil
	}
	return pts, []*mtproto.Chat{chat}
}

// ScheduledService serves the scheduled-message inbox.
// https://core.telegram.org/api/scheduled-messages
type ScheduledService struct {
	mtproto.UnimplementedRPCScheduledMessagesServer
	Msg msg_client.MsgClient
}

func (s ScheduledService) MessagesGetScheduledHistory(ctx context.Context, in *mtproto.TLMessagesGetScheduledHistory) (*mtproto.Messages_Messages, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	page, err := chatbox.Shared().ScheduledHistory(ctx, self, peer, in.GetHash())
	if err != nil {
		return nil, err
	}
	return withChannelCard(ctx, self, peer, page), nil
}

func (s ScheduledService) MessagesGetScheduledMessages(ctx context.Context, in *mtproto.TLMessagesGetScheduledMessages) (*mtproto.Messages_Messages, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	page, err := chatbox.Shared().ScheduledByID(ctx, self, peer, in.GetId())
	if err != nil {
		return nil, err
	}
	return withChannelCard(ctx, self, peer, page), nil
}

func (s ScheduledService) MessagesSendScheduledMessages(ctx context.Context, in *mtproto.TLMessagesSendScheduledMessages) (*mtproto.Updates, error) {
	self, auth := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	taken, err := chatbox.Shared().TakeScheduled(ctx, self, peer, in.GetId())
	if err != nil {
		return nil, err
	}
	if s.Msg == nil {
		chatbox.Shared().RestoreScheduled(ctx, self, peer, taken)
		return nil, mtproto.ErrInternalServerError
	}
	boxes := make([]*msg.OutboxMessage, 0, len(taken))
	schedIDs := make([]int32, 0, len(taken))
	now := int32(time.Now().Unix())
	for _, m := range taken {
		schedIDs = append(schedIDs, m.GetId())
		send := proto.Clone(m).(*mtproto.Message)
		send.FromScheduled = true
		send.Date = now
		send.Id = 0
		boxes = append(boxes, msg.MakeTLOutboxMessage(&msg.OutboxMessage{
			RandomId: randID(),
			Message:  send,
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
		chatbox.Shared().RestoreScheduled(ctx, self, peer, taken)
		return nil, err
	}
	var sent []int32
	if ups != nil {
		for _, u := range ups.Updates {
			if u.GetMessage_MESSAGE() != nil {
				sent = append(sent, u.GetMessage_MESSAGE().GetId())
			}
		}
		del := chatbox.DeleteScheduledUpdate(peer, schedIDs, sent)
		ups.Updates = append([]*mtproto.Update{del}, ups.Updates...)
	}
	return ups, nil
}

func (s ScheduledService) MessagesDeleteScheduledMessages(ctx context.Context, in *mtproto.TLMessagesDeleteScheduledMessages) (*mtproto.Updates, error) {
	self, _ := selfID(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	return chatbox.Shared().DeleteScheduled(ctx, self, peer, in.GetId())
}

func withChannelCard(ctx context.Context, self int64, peer *mtproto.PeerUtil, page *mtproto.Messages_Messages) *mtproto.Messages_Messages {
	if page == nil || peer == nil || !peer.IsChannel() || page.GetPredicateName() == mtproto.Predicate_messages_messagesNotModified {
		return page
	}
	pts, chats := channelCard(ctx, self, peer.PeerId)
	if pts == 0 && len(chats) == 0 {
		return page
	}
	return chatbox.PageMessages(peer, pts, page.Messages, chats)
}

func randID() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 1
	}
	v := int64(binary.BigEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}

// WebPageService serves link cards.
type WebPageService struct {
	mtproto.UnimplementedRPCWebPageServer
}

// MessagesGetWebPage8D9692A3 is messages.getWebPage#8d9692a3.
// https://core.telegram.org/method/messages.getWebPage
func (WebPageService) MessagesGetWebPage8D9692A3(ctx context.Context, in *mtproto.TLMessagesGetWebPage8D9692A3) (*mtproto.Messages_WebPage, error) {
	page, err := chatbox.WebPage(in.GetUrl(), in.GetHash())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesWebPage(&mtproto.Messages_WebPage{
		Webpage: page,
		Chats:   []*mtproto.Chat{},
		Users:   []*mtproto.User{},
	}).To_Messages_WebPage(), nil
}

// MessagesGetWebPage32CA8F91 is the older messages.getWebPage that returns WebPage.
func (WebPageService) MessagesGetWebPage32CA8F91(ctx context.Context, in *mtproto.TLMessagesGetWebPage32CA8F91) (*mtproto.WebPage, error) {
	return chatbox.WebPage(in.GetUrl(), in.GetHash())
}

// MessagesGetWebPagePreview570D6F6F is messages.getWebPagePreview#570d6f6f.
// https://core.telegram.org/method/messages.getWebPagePreview
func (WebPageService) MessagesGetWebPagePreview570D6F6F(ctx context.Context, in *mtproto.TLMessagesGetWebPagePreview570D6F6F) (*mtproto.Messages_WebPagePreview, error) {
	return mtproto.MakeTLMessagesWebPagePreview(&mtproto.Messages_WebPagePreview{
		Media: chatbox.WebPagePreview(in.GetMessage(), in.GetEntities()),
		Chats: []*mtproto.Chat{},
		Users: []*mtproto.User{},
	}).To_Messages_WebPagePreview(), nil
}

// MessagesGetWebPagePreview8B68B0CC is the older preview that returns MessageMedia.
func (WebPageService) MessagesGetWebPagePreview8B68B0CC(ctx context.Context, in *mtproto.TLMessagesGetWebPagePreview8B68B0CC) (*mtproto.MessageMedia, error) {
	return chatbox.WebPagePreview(in.GetMessage(), in.GetEntities()), nil
}
