package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/postsearch"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

// ChannelsSearchPosts
// channels.searchPosts#f2c4f24d flags:# hashtag:flags.0?string query:flags.1?string offset_rate:int offset_peer:InputPeer offset_id:int limit:int allow_paid_stars:flags.2?long = messages.Messages;
//
// allow_paid_stars is ignored. A search never spends Stars.
func (c *MessagesCore) ChannelsSearchPosts(in *mtproto.TLChannelsSearchPosts) (*mtproto.Messages_Messages, error) {
	needle := in.GetQuery().GetValue()
	if tag := in.GetHashtag_STRING(); tag != "" {
		needle = tag
	}
	if tag := in.GetHashtag_FLAGSTRING().GetValue(); tag != "" {
		needle = tag
	}
	var peerID int64
	if peer := mtproto.FromInputPeer2(c.MD.UserId, in.GetOffsetPeer()); peer != nil && peer.PeerType == mtproto.PEER_CHANNEL {
		peerID = peer.PeerId
	}
	res, err := channel.Shared().SearchPosts(c.ctx, c.MD.UserId, channel.PostQuery{
		Needle:     needle,
		OffsetRate: in.GetOffsetRate(),
		OffsetPeer: peerID,
		OffsetID:   in.GetOffsetId(),
		Limit:      in.GetLimit(),
	})
	if err != nil {
		c.Logger.Errorf("channels.searchPosts - error: %v", err)
		return nil, err
	}
	if res != nil {
		res.SearchFlood = postsearch.Shared().Spend(c.ctx, c.MD.UserId)
		c.fillPostChats(res)
		c.fillPostUsers(res)
		if sealed := sealMessages(res, c.layer()); sealed != nil {
			res = sealed
		}
	}
	return res, nil
}

func (c *MessagesCore) layer() int32 {
	if c != nil && c.MD != nil && c.MD.Layer > 0 {
		return c.MD.Layer
	}
	return mtproto.Layer
}

// sealMessages re-encodes the reply at the client layer and decodes it
// again. A chat or user that does not survive that round trip is replaced
// with a smaller object that does, so the vectors are not dropped later
// when the session serializes the same layer.
func sealMessages(res *mtproto.Messages_Messages, layer int32) *mtproto.Messages_Messages {
	if res == nil {
		return nil
	}
	if got := roundTripMessages(res, layer); vectorsOK(got) {
		return got
	}
	ids := map[int64]struct{}{}
	for _, msg := range res.GetMessages() {
		if id := msg.GetPeerId().GetChannelId(); id != 0 {
			ids[id] = struct{}{}
		}
	}
	var chats []*mtproto.Chat
	for id := range ids {
		chats = append(chats, mtproto.MakeTLChannel(&mtproto.Chat{
			Broadcast: true,
			Id:        id,
			Title:     "",
			Photo:     mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto(),
		}).To_Chat())
	}
	var users []*mtproto.User
	seen := map[int64]struct{}{}
	for _, user := range res.GetUsers() {
		if user == nil || user.GetId() == 0 {
			continue
		}
		if _, ok := seen[user.GetId()]; ok {
			continue
		}
		seen[user.GetId()] = struct{}{}
		users = append(users, mtproto.MakeTLUserEmpty(&mtproto.User{Id: user.GetId()}).To_User())
	}
	res.Chats = chats
	res.Users = users
	if got := roundTripMessages(res, layer); got != nil {
		return got
	}
	return res
}

func vectorsOK(res *mtproto.Messages_Messages) bool {
	return res != nil && len(res.GetMessages()) > 0 && len(res.GetChats()) > 0 && len(res.GetUsers()) > 0
}

func roundTripMessages(res *mtproto.Messages_Messages, layer int32) *mtproto.Messages_Messages {
	if layer <= 0 {
		layer = mtproto.Layer
	}
	buf := mtproto.NewEncodeBuf(8192)
	if err := res.Encode(buf, layer); err != nil {
		return nil
	}
	d := mtproto.NewDecodeBuf(buf.GetBuf())
	obj := d.Object()
	if err := d.GetError(); err != nil || obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case *mtproto.TLMessagesMessagesSlice:
		return v.To_Messages_Messages()
	case *mtproto.TLMessagesChannelMessages:
		return v.To_Messages_Messages()
	case *mtproto.TLMessagesMessages:
		return v.To_Messages_Messages()
	default:
		return nil
	}
}

func (c *MessagesCore) fillPostChats(res *mtproto.Messages_Messages) {
	if res == nil || len(res.GetMessages()) == 0 {
		return
	}
	seen := map[int64]bool{}
	var ids []int64
	for _, msg := range res.Messages {
		id := msg.GetPeerId().GetChannelId()
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return
	}
	if chats := channel.Shared().ChatsFor(c.ctx, c.MD.UserId, ids...); len(chats) > 0 {
		res.Chats = chats
	}
	have := map[int64]bool{}
	for _, chat := range res.Chats {
		if chat != nil {
			have[chat.GetId()] = true
		}
	}
	for _, id := range ids {
		if have[id] {
			continue
		}
		res.Chats = append(res.Chats, mtproto.MakeTLChannel(&mtproto.Chat{
			Broadcast: true,
			Id:        id,
			Title:     "",
			Photo:     mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto(),
		}).To_Chat())
	}
}

func (c *MessagesCore) fillPostUsers(res *mtproto.Messages_Messages) {
	if c == nil || res == nil {
		return
	}
	seen := map[int64]struct{}{}
	var ids []int64
	add := func(id int64) {
		if id == 0 {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	if c.MD != nil {
		add(c.MD.UserId)
	}
	for _, user := range res.Users {
		add(user.GetId())
	}
	for _, msg := range res.Messages {
		add(msg.GetFromId().GetUserId())
	}
	if c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.UserClient != nil && c.MD != nil && len(ids) > 0 {
		users, err := c.svcCtx.Dao.UserClient.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: ids})
		if err == nil && users != nil {
			if got := users.GetUserListByIdList(c.MD.UserId, ids...); len(got) > 0 {
				res.Users = got
			}
		}
	}
	have := map[int64]bool{}
	for _, user := range res.Users {
		if user != nil {
			have[user.GetId()] = true
		}
	}
	for _, id := range ids {
		if have[id] {
			continue
		}
		res.Users = append(res.Users, mtproto.MakeTLUserEmpty(&mtproto.User{Id: id}).To_User())
	}
}
