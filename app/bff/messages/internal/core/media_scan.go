package core

import (
	"math"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/searchbox"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

const (
	mediaPage = int32(100)
	mediaCap  = 2000
)

// testMediaRows replaces the message service in handler tests.
var testMediaRows func(peerType int32, peerID int64, kind searchbox.MediaKind) ([]searchbox.Row, bool, error)

func nowUnix() int32 { return int32(time.Now().Unix()) }

func (c *MessagesCore) requireUser() error {
	if c.MD == nil || c.MD.UserId == 0 {
		return mtproto.ErrUserIdInvalid
	}
	if c.MD.IsBot {
		return mtproto.ErrBotMethodInvalid
	}
	return nil
}

func (c *MessagesCore) peerOrInvalid(in *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	peer := mtproto.FromInputPeer2(c.MD.UserId, in)
	if peer == nil || peer.IsEmpty() || peer.PeerId == 0 && !peer.IsSelf() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsSelf() && peer.PeerId == 0 {
		peer.PeerId = c.MD.UserId
	}
	if !(peer.IsUser() || peer.IsChat() || peer.IsChannel()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return peer, nil
}

func (c *MessagesCore) scanMedia(peer *mtproto.PeerUtil, kind searchbox.MediaKind, saved *mtproto.PeerUtil) ([]searchbox.Row, bool, error) {
	if testMediaRows != nil {
		rows, truncated, err := testMediaRows(peer.PeerType, peer.PeerId, kind)
		if err != nil {
			return nil, false, err
		}
		return filterSaved(rows, saved), truncated, nil
	}
	if kind.Pinned {
		return c.scanPinned(peer, saved)
	}
	return c.scanByType(peer, kind.Type, saved)
}

func (c *MessagesCore) scanByType(peer *mtproto.PeerUtil, media int32, saved *mtproto.PeerUtil) ([]searchbox.Row, bool, error) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, false, nil
	}
	offset := int32(math.MaxInt32)
	var rows []searchbox.Row
	for len(rows) < mediaCap {
		got, err := c.svcCtx.Dao.MessageSearchByMediaType(c.ctx, &messagepb.TLMessageSearchByMediaType{
			UserId:    c.MD.UserId,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			MediaType: media,
			Offset:    offset,
			Limit:     mediaPage,
		})
		if err != nil {
			return nil, false, err
		}
		list := got.GetBoxList()
		if len(list) == 0 {
			return rows, false, nil
		}
		for _, box := range list {
			row := rowFromBox(box)
			if row.ID > 0 && row.ID < offset {
				offset = row.ID
			}
			if saved != nil && !savedPeerMatch(row.Message, saved) {
				continue
			}
			rows = append(rows, row)
		}
		if len(list) < int(mediaPage) {
			return rows, false, nil
		}
		if len(rows) >= mediaCap {
			return rows, true, nil
		}
	}
	return rows, true, nil
}

func (c *MessagesCore) scanPinned(peer *mtproto.PeerUtil, saved *mtproto.PeerUtil) ([]searchbox.Row, bool, error) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, false, nil
	}
	got, err := c.svcCtx.Dao.MessageSearchByPinned(c.ctx, &messagepb.TLMessageSearchByPinned{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
	})
	if err != nil {
		return nil, false, err
	}
	var rows []searchbox.Row
	for _, box := range got.GetBoxList() {
		row := rowFromBox(box)
		if saved != nil && !savedPeerMatch(row.Message, saved) {
			continue
		}
		rows = append(rows, row)
	}
	return rows, false, nil
}

func (c *MessagesCore) scanHistory(peer *mtproto.PeerUtil, pages int) ([]searchbox.Row, error) {
	if testMediaRows != nil {
		rows, _, err := testMediaRows(peer.PeerType, peer.PeerId, searchbox.MediaKind{})
		return rows, err
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, nil
	}
	offset := int32(math.MaxInt32)
	var rows []searchbox.Row
	for i := 0; i < pages; i++ {
		got, err := c.svcCtx.Dao.MessageGetHistoryMessages(c.ctx, &messagepb.TLMessageGetHistoryMessages{
			UserId:   c.MD.UserId,
			PeerType: peer.PeerType,
			PeerId:   peer.PeerId,
			OffsetId: offset,
			Limit:    mediaPage,
		})
		if err != nil {
			return nil, err
		}
		list := got.GetDatas()
		if len(list) == 0 {
			break
		}
		for _, box := range list {
			row := rowFromBox(box)
			if row.ID > 0 && row.ID < offset {
				offset = row.ID
			}
			rows = append(rows, row)
		}
		if len(list) < int(mediaPage) {
			break
		}
	}
	return rows, nil
}

func (c *MessagesCore) scanSent(kind searchbox.MediaKind) ([]searchbox.Row, error) {
	if testMediaRows != nil {
		rows, _, err := testMediaRows(0, 0, kind)
		return rows, err
	}
	peers := []*mtproto.PeerUtil{{PeerType: mtproto.PEER_SELF, PeerId: c.MD.UserId, SelfId: c.MD.UserId}}
	if c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.DialogClient != nil {
		got, err := c.svcCtx.Dao.DialogGetDialogs(c.ctx, &dialog.TLDialogGetDialogs{
			UserId:        c.MD.UserId,
			ExcludePinned: mtproto.BoolFalse,
			FolderId:      0,
		})
		if err == nil && got != nil {
			for _, row := range got.GetDatas() {
				if row == nil || row.GetDialog() == nil || row.GetDialog().GetPeer() == nil {
					continue
				}
				raw := row.GetDialog().GetPeer()
				switch raw.GetPredicateName() {
				case mtproto.Predicate_peerUser, mtproto.Predicate_peerChat, mtproto.Predicate_peerChannel:
				default:
					continue
				}
				peer := mtproto.FromPeer(raw)
				peers = append(peers, peer)
				if len(peers) >= 30 {
					break
				}
			}
		}
	}
	var rows []searchbox.Row
	for _, peer := range peers {
		var part []searchbox.Row
		var err error
		if kind.Pinned {
			part, _, err = c.scanPinned(peer, nil)
		} else {
			part, _, err = c.scanByType(peer, kind.Type, nil)
		}
		if err != nil {
			c.Logger.Errorf("messages.searchSentMedia - peer %d: %v", peer.PeerId, err)
			continue
		}
		for _, row := range part {
			if row.Sender == c.MD.UserId {
				rows = append(rows, row)
			}
		}
	}
	sortRows(rows)
	return rows, nil
}

func sortRows(rows []searchbox.Row) {
	for i := 1; i < len(rows); i++ {
		j := i
		for j > 0 && (rows[j].Date > rows[j-1].Date || (rows[j].Date == rows[j-1].Date && rows[j].ID > rows[j-1].ID)) {
			rows[j], rows[j-1] = rows[j-1], rows[j]
			j--
		}
	}
}

func rowFromBox(box *mtproto.MessageBox) searchbox.Row {
	if box == nil {
		return searchbox.Row{}
	}
	msg := box.GetMessage()
	row := searchbox.Row{
		ID:      box.GetMessageId(),
		Sender:  box.GetSenderUserId(),
		Message: msg,
	}
	if msg == nil {
		return row
	}
	row.Date = msg.GetDate()
	row.Text = msg.GetMessage()
	if msg.GetMedia().GetPredicateName() == mtproto.Predicate_messageMediaGeoLive {
		row.Live = true
		row.Period = msg.GetMedia().GetPeriod()
	}
	if top := msg.GetReplyTo().GetReplyToTopId(); top != nil {
		row.TopID = top.GetValue()
	} else if box.GetReplyToTopId() != 0 {
		row.TopID = box.GetReplyToTopId()
	}
	return row
}

func filterSaved(rows []searchbox.Row, saved *mtproto.PeerUtil) []searchbox.Row {
	if saved == nil {
		return rows
	}
	out := make([]searchbox.Row, 0, len(rows))
	for _, row := range rows {
		if savedPeerMatch(row.Message, saved) {
			out = append(out, row)
		}
	}
	return out
}

func savedPeerMatch(msg *mtproto.Message, want *mtproto.PeerUtil) bool {
	if want == nil {
		return true
	}
	if msg == nil || msg.GetSavedPeerId() == nil {
		return false
	}
	raw := msg.GetSavedPeerId()
	switch raw.GetPredicateName() {
	case mtproto.Predicate_peerUser:
		return want.IsUser() && raw.GetUserId() == want.PeerId
	case mtproto.Predicate_peerChat:
		return want.IsChat() && raw.GetChatId() == want.PeerId
	case mtproto.Predicate_peerChannel:
		return want.IsChannel() && raw.GetChannelId() == want.PeerId
	default:
		return false
	}
}

func (c *MessagesCore) savedPeer(in *mtproto.InputPeer) *mtproto.PeerUtil {
	if in == nil {
		return nil
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in)
	if peer == nil || peer.IsEmpty() || peer.PeerId == 0 {
		return nil
	}
	return peer
}

func (c *MessagesCore) peersOf(msgs []*mtproto.Message) (users []*mtproto.User, chats []*mtproto.Chat) {
	users = []*mtproto.User{}
	chats = []*mtproto.Chat{}
	if len(msgs) == 0 || c.svcCtx == nil || c.svcCtx.Dao == nil {
		return users, chats
	}
	var userIDs, chatIDs, channelIDs []int64
	addUser := func(id int64) {
		if id != 0 {
			userIDs = append(userIDs, id)
		}
	}
	for _, msg := range msgs {
		if msg == nil {
			continue
		}
		addUser(msg.GetFromId().GetUserId())
		switch msg.GetPeerId().GetPredicateName() {
		case mtproto.Predicate_peerUser:
			addUser(msg.GetPeerId().GetUserId())
		case mtproto.Predicate_peerChat:
			chatIDs = append(chatIDs, msg.GetPeerId().GetChatId())
		case mtproto.Predicate_peerChannel:
			channelIDs = append(channelIDs, msg.GetPeerId().GetChannelId())
		}
	}
	addUser(c.MD.UserId)
	if c.svcCtx.Dao.UserClient != nil && len(userIDs) > 0 {
		got, err := c.svcCtx.Dao.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: userIDs})
		if err == nil && got != nil {
			users = got.GetUserListByIdList(c.MD.UserId, userIDs...)
			if users == nil {
				users = []*mtproto.User{}
			}
		}
	}
	if c.svcCtx.Dao.ChatClient != nil && len(chatIDs) > 0 {
		got, err := c.svcCtx.Dao.ChatClient.Client().ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{
			SelfId: c.MD.UserId,
			IdList: chatIDs,
		})
		if err == nil && got != nil {
			chats = append(chats, got.GetChatListByIdList(c.MD.UserId, chatIDs...)...)
		}
	}
	for _, id := range channelIDs {
		if chat := channel.Shared().VisibleChat(c.ctx, c.MD.UserId, id); chat != nil {
			chats = append(chats, chat)
		}
	}
	return users, chats
}

func messagesOf(rows []searchbox.Row) []*mtproto.Message {
	out := make([]*mtproto.Message, 0, len(rows))
	for _, row := range rows {
		if row.Message != nil {
			out = append(out, row.Message)
			continue
		}
		out = append(out, &mtproto.Message{Id: row.ID, Date: row.Date, Message: row.Text})
	}
	return out
}
