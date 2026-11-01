package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/saved"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (c *SavedMessageDialogsCore) requireUser() error {
	if c.MD == nil || c.MD.UserId == 0 {
		return mtproto.ErrUserIdInvalid
	}
	if c.MD.IsBot {
		return mtproto.ErrBotMethodInvalid
	}
	return nil
}

func (c *SavedMessageDialogsCore) savedPeer(in *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in)
	if peer.PeerType == mtproto.PEER_SELF {
		peer.PeerType = mtproto.PEER_USER
		peer.PeerId = c.MD.UserId
	}
	if !saved.SavedPeer(peer) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return peer, nil
}

func inputChannel(p *mtproto.PeerUtil) *mtproto.InputChannel {
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  p.PeerId,
		AccessHash: p.AccessHash,
	}).To_InputChannel()
}

func (c *SavedMessageDialogsCore) requireMonoforum(in *mtproto.InputPeer) error {
	if in == nil {
		return mtproto.ErrParentPeerInvalid
	}
	peer := mtproto.FromInputPeer2(c.MD.UserId, in)
	if peer == nil || peer.PeerType != mtproto.PEER_CHANNEL || peer.PeerId == 0 {
		return mtproto.ErrParentPeerInvalid
	}
	return channel.Shared().RequireMonoforumAdmin(c.ctx, c.MD.UserId, inputChannel(peer))
}

func dateValue(v *wrapperspb.Int32Value) int32 {
	if v == nil || v.Value < 0 {
		return 0
	}
	return v.Value
}

func (c *SavedMessageDialogsCore) querySaved(exclude bool, offsetID, limit int32) (*dialog.SavedDialogList, error) {
	return c.svcCtx.Dao.DialogGetSavedDialogs(c.ctx, &dialog.TLDialogGetSavedDialogs{
		UserId:        c.MD.UserId,
		ExcludePinned: mtproto.ToBool(exclude),
		OffsetId:      offsetID,
		Limit:         limit,
	})
}

func (c *SavedMessageDialogsCore) lookupSaved(peer *mtproto.PeerUtil) (*mtproto.SavedDialog, error) {
	list, err := c.svcCtx.Dao.DialogGetSavedDialogs(c.ctx, &dialog.TLDialogGetSavedDialogs{
		UserId:     c.MD.UserId,
		OffsetPeer: peer,
		Limit:      dialog.SavedLookupLimit,
	})
	if err != nil || list == nil || len(list.Dialogs) == 0 {
		return nil, err
	}
	return list.Dialogs[0], nil
}

func (c *SavedMessageDialogsCore) touchSaved(peer *mtproto.PeerUtil, top int32) error {
	_, err := c.svcCtx.Dao.DialogGetSavedDialogs(c.ctx, &dialog.TLDialogGetSavedDialogs{
		UserId:     c.MD.UserId,
		OffsetPeer: peer,
		OffsetId:   top,
		Limit:      dialog.SavedTouchLimit,
	})
	return err
}

func rowsOf(dialogs []*mtproto.SavedDialog) []saved.Row {
	out := make([]saved.Row, 0, len(dialogs))
	for _, d := range dialogs {
		if d == nil || d.Peer == nil {
			continue
		}
		p := mtproto.FromPeer(d.Peer)
		out = append(out, saved.Row{
			Pinned:   d.Pinned,
			PeerType: p.PeerType,
			PeerID:   p.PeerId,
			Top:      d.TopMessage,
		})
	}
	return out
}

// allSaved loads every saved dialog of this user for the cache hash.
// incomplete is set when the scan stops at the cap.
func (c *SavedMessageDialogsCore) allSaved(exclude bool) (rows []saved.Row, count int32, incomplete bool, err error) {
	var cursor int32
	for page := 0; page < 50; page++ {
		list, err := c.querySaved(exclude, cursor, saved.PageCap)
		if err != nil {
			return nil, 0, false, err
		}
		if list == nil || len(list.Dialogs) == 0 {
			break
		}
		rows = append(rows, rowsOf(list.Dialogs)...)
		last := list.Dialogs[len(list.Dialogs)-1].GetTopMessage()
		if last == 0 || last == cursor {
			break
		}
		cursor = last
		if int32(len(list.Dialogs)) < saved.PageCap {
			break
		}
		if page == 49 {
			incomplete = true
		}
	}
	return rows, int32(len(rows)), incomplete, nil
}

func emptySaved(full bool, count int32) *mtproto.Messages_SavedDialogs {
	if full {
		return mtproto.MakeTLMessagesSavedDialogs(&mtproto.Messages_SavedDialogs{
			Dialogs:  []*mtproto.SavedDialog{},
			Messages: []*mtproto.Message{},
			Chats:    []*mtproto.Chat{},
			Users:    []*mtproto.User{},
		}).To_Messages_SavedDialogs()
	}
	return mtproto.MakeTLMessagesSavedDialogsSlice(&mtproto.Messages_SavedDialogs{
		Count:    count,
		Dialogs:  []*mtproto.SavedDialog{},
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}).To_Messages_SavedDialogs()
}

func (c *SavedMessageDialogsCore) packSaved(dialogs []*mtproto.SavedDialog, count int32, full bool) (*mtproto.Messages_SavedDialogs, error) {
	if len(dialogs) == 0 {
		return emptySaved(full, count), nil
	}
	ids := make([]int32, 0, len(dialogs))
	for _, d := range dialogs {
		if d != nil && d.TopMessage != 0 {
			ids = append(ids, d.TopMessage)
		}
	}
	var msgs []*mtproto.Message
	userSet := map[int64]struct{}{}
	chatSet := map[int64]struct{}{}
	channelSet := map[int64]struct{}{}
	add := func(p *mtproto.PeerUtil) {
		if p == nil {
			return
		}
		switch p.PeerType {
		case mtproto.PEER_SELF:
			userSet[c.MD.UserId] = struct{}{}
		case mtproto.PEER_USER:
			if p.PeerId != 0 {
				userSet[p.PeerId] = struct{}{}
			}
		case mtproto.PEER_CHAT:
			if p.PeerId != 0 {
				chatSet[p.PeerId] = struct{}{}
			}
		case mtproto.PEER_CHANNEL:
			if p.PeerId != 0 {
				channelSet[p.PeerId] = struct{}{}
			}
		}
	}
	for _, d := range dialogs {
		if d != nil && d.Peer != nil {
			add(mtproto.FromPeer(d.Peer))
		}
	}
	if len(ids) > 0 {
		boxList, err := c.svcCtx.Dao.MessageGetUserMessageList(c.ctx, &message.TLMessageGetUserMessageList{
			UserId: c.MD.UserId,
			IdList: ids,
		})
		if err != nil {
			c.Logger.Errorf("saved dialogs messages - error: %v", err)
			return nil, err
		}
		if boxList != nil {
			boxList.Visit(c.MD.UserId,
				func(messageList []*mtproto.Message) { msgs = messageList },
				func(userIdList []int64) {
					for _, id := range userIdList {
						userSet[id] = struct{}{}
					}
				},
				func(chatIdList []int64) {
					for _, id := range chatIdList {
						chatSet[id] = struct{}{}
					}
				},
				func(channelIdList []int64) {
					for _, id := range channelIdList {
						channelSet[id] = struct{}{}
					}
				})
		}
	}
	users := c.usersByID(keys(userSet))
	chats := c.chatsByID(keys(chatSet))
	chats = append(chats, c.channelChats(keys(channelSet))...)
	if msgs == nil {
		msgs = []*mtproto.Message{}
	}
	if full {
		return mtproto.MakeTLMessagesSavedDialogs(&mtproto.Messages_SavedDialogs{
			Dialogs:  dialogs,
			Messages: msgs,
			Chats:    chats,
			Users:    users,
		}).To_Messages_SavedDialogs(), nil
	}
	return mtproto.MakeTLMessagesSavedDialogsSlice(&mtproto.Messages_SavedDialogs{
		Count:    count,
		Dialogs:  dialogs,
		Messages: msgs,
		Chats:    chats,
		Users:    users,
	}).To_Messages_SavedDialogs(), nil
}

func keys(m map[int64]struct{}) []int64 {
	if len(m) == 0 {
		return nil
	}
	out := make([]int64, 0, len(m))
	for id := range m {
		out = append(out, id)
	}
	return out
}

func (c *SavedMessageDialogsCore) usersByID(ids []int64) []*mtproto.User {
	if len(ids) == 0 {
		return []*mtproto.User{}
	}
	users, err := c.svcCtx.Dao.UserGetMutableUsers(c.ctx, &userpb.TLUserGetMutableUsers{Id: ids})
	if err != nil || users == nil {
		c.Logger.Errorf("saved dialogs users - error: %v", err)
		return []*mtproto.User{}
	}
	return mtproto.ToSafeUsers(users.GetUserListByIdList(c.MD.UserId, ids...))
}

func (c *SavedMessageDialogsCore) chatsByID(ids []int64) []*mtproto.Chat {
	if len(ids) == 0 {
		return nil
	}
	chats, err := c.svcCtx.Dao.ChatGetChatListByIdList(c.ctx, &chatpb.TLChatGetChatListByIdList{IdList: ids})
	if err != nil || chats == nil {
		c.Logger.Errorf("saved dialogs chats - error: %v", err)
		return nil
	}
	return mtproto.ToSafeChats(chats.GetChatListByIdList(c.MD.UserId, ids...))
}

func (c *SavedMessageDialogsCore) channelChats(ids []int64) []*mtproto.Chat {
	if len(ids) == 0 {
		return nil
	}
	return channel.Shared().ChatsFor(c.ctx, c.MD.UserId, ids...)
}
