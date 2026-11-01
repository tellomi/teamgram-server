package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/saved"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
)

// MessagesGetSavedHistory
// messages.getSavedHistory#998ab009 flags:# parent_peer:flags.0?InputPeer peer:InputPeer offset_id:int offset_date:int add_offset:int limit:int max_id:int min_id:int hash:long = messages.Messages;
// https://core.telegram.org/method/messages.getSavedHistory
func (c *SavedMessageDialogsCore) MessagesGetSavedHistory(in *mtproto.TLMessagesGetSavedHistory) (*mtproto.Messages_Messages, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	if saved.ParentSpecified(in.GetParentPeer()) {
		if err := c.requireMonoforum(in.GetParentPeer()); err != nil {
			c.Logger.Errorf("messages.getSavedHistory - error: %v", err)
			return nil, err
		}
		return emptyMessages(), nil
	}
	peer, err := c.savedPeer(in.GetPeer())
	if err != nil {
		c.Logger.Errorf("messages.getSavedHistory - error: %v", err)
		return nil, err
	}
	limit := saved.NormalizeLimit(in.GetLimit(), saved.HistoryCap)
	boxList, err := c.svcCtx.Dao.MessageGetSavedHistoryMessages(c.ctx, &message.TLMessageGetSavedHistoryMessages{
		UserId:     c.MD.UserId,
		PeerType:   peer.PeerType,
		PeerId:     peer.PeerId,
		OffsetId:   in.OffsetId,
		OffsetDate: in.OffsetDate,
		AddOffset:  in.AddOffset,
		Limit:      limit,
		MaxId:      in.MaxId,
		MinId:      in.MinId,
		Hash:       in.Hash,
	})
	if err != nil {
		c.Logger.Errorf("messages.getSavedHistory - error: %v", err)
		return nil, err
	}
	if boxList == nil {
		boxList = &mtproto.MessageBoxList{}
	}
	var (
		messages []*mtproto.Message
		users    []*mtproto.User
		chats    []*mtproto.Chat
	)
	userSet := map[int64]struct{}{}
	chatSet := map[int64]struct{}{}
	channelSet := map[int64]struct{}{}
	addPeer(peer, c.MD.UserId, userSet, chatSet, channelSet)
	boxList.Visit(c.MD.UserId,
		func(messageList []*mtproto.Message) { messages = messageList },
		func(ids []int64) {
			for _, id := range ids {
				userSet[id] = struct{}{}
			}
		},
		func(ids []int64) {
			for _, id := range ids {
				chatSet[id] = struct{}{}
			}
		},
		func(ids []int64) {
			for _, id := range ids {
				channelSet[id] = struct{}{}
			}
		})
	ids := make([]int32, 0, len(messages))
	for _, msg := range messages {
		if msg != nil {
			ids = append(ids, msg.GetId())
		}
	}
	if in.GetHash() != 0 && in.GetHash() == saved.HashIDs(ids) {
		return mtproto.MakeTLMessagesMessagesNotModified(&mtproto.Messages_Messages{
			Count: boxList.Count,
		}).To_Messages_Messages(), nil
	}
	users = c.usersByID(keys(userSet))
	chats = append(c.chatsByID(keys(chatSet)), c.channelChats(keys(channelSet))...)
	if messages == nil {
		messages = []*mtproto.Message{}
	}
	if chats == nil {
		chats = []*mtproto.Chat{}
	}
	if int32(len(messages)) == limit {
		return mtproto.MakeTLMessagesMessagesSlice(&mtproto.Messages_Messages{
			Count:    boxList.Count,
			Messages: messages,
			Users:    users,
			Chats:    chats,
		}).To_Messages_Messages(), nil
	}
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: messages,
		Users:    users,
		Chats:    chats,
	}).To_Messages_Messages(), nil
}

func emptyMessages() *mtproto.Messages_Messages {
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: []*mtproto.Message{},
		Users:    []*mtproto.User{},
		Chats:    []*mtproto.Chat{},
	}).To_Messages_Messages()
}

func addPeer(p *mtproto.PeerUtil, self int64, users, chats, channels map[int64]struct{}) {
	if p == nil {
		return
	}
	switch p.PeerType {
	case mtproto.PEER_SELF:
		users[self] = struct{}{}
	case mtproto.PEER_USER:
		if p.PeerId != 0 {
			users[p.PeerId] = struct{}{}
		}
	case mtproto.PEER_CHAT:
		if p.PeerId != 0 {
			chats[p.PeerId] = struct{}{}
		}
	case mtproto.PEER_CHANNEL:
		if p.PeerId != 0 {
			channels[p.PeerId] = struct{}{}
		}
	}
}
