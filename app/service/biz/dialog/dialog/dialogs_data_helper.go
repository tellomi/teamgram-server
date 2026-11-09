// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package dialog

import (
	"github.com/teamgram/proto/mtproto"
)

type DialogsDataHelper struct {
	Dialogs  []*mtproto.Dialog  `json:"dialogs"`
	Messages []*mtproto.Message `json:"messages"`
	Chats    []*mtproto.Chat    `json:"chats"`
	Users    []*mtproto.User    `json:"users"`
}

func (m *DialogsDataHelper) fix() {
	var (
		dialogs  = make([]*mtproto.Dialog, 0, len(m.Dialogs))
		messages = make([]*mtproto.Message, 0, len(m.Messages))
		chats    = make([]*mtproto.Chat, 0, len(m.Chats))
		users    = make([]*mtproto.User, 0, len(m.Users))
	)

	for _, dlg := range m.Dialogs {
		var (
			topMessage *mtproto.Message
		)

		if dlg.TopMessage == 0 {
			continue
		}

		p := mtproto.FromPeer(dlg.Peer)
		for _, msg := range m.Messages {
			to := mtproto.FromPeer(msg.PeerId)
			if to.PeerType == p.PeerType && to.PeerId == p.PeerId && dlg.TopMessage == msg.Id {
				topMessage = msg
				break
			}
		}

		if topMessage != nil {
			found := false
			switch p.PeerType {
			case mtproto.PEER_USER:
				for _, v := range m.Users {
					if v.Id == p.PeerId {
						//if v.Deleted {
						//	topMessage = nil
						//} else {
						users = append(users, v)
						//}
						found = true
						break
					}
				}
			case mtproto.PEER_CHAT:
				for _, v := range m.Chats {
					if v.Id == p.PeerId {
						// TODO: check chatEmpty/chatForbidden
						//if v.Deactivated || v.Left {
						//	topMessage = nil
						//} else {
						chats = append(chats, v)
						//}
						found = true
						break
					}
				}
			case mtproto.PEER_CHANNEL:
				for _, v := range m.Chats {
					if v.Id == p.PeerId {
						// TODO: check channelForbidden
						//if v.Left {
						//	topMessage = nil
						//} else {
						chats = append(chats, v)
						//}
						found = true
						break
					}
				}
			}
			if !found {
				topMessage = nil
			}
		}
		if topMessage != nil {
			dialogs = append(dialogs, dlg)
			messages = append(messages, topMessage)
		}
	}

	m.Dialogs = dialogs
	m.Messages = messages
	m.Users = users
	m.Chats = chats
}

// ToMessagesDialogs builds the messages.getDialogs reply.
// count is the number of dialogs that can be returned, measured after fix().
// exhausted means this page already holds every dialog still below the
// cursor, including a first page whose total fits in limit. That reply is
// messages.dialogs. A page with more rows below it is messages.dialogsSlice.
func (m *DialogsDataHelper) ToMessagesDialogs(count int32, exhausted bool) *mtproto.Messages_Dialogs {
	before := len(m.Dialogs)
	m.fix()
	dropped := int32(before - len(m.Dialogs))
	if dropped > 0 && count >= dropped {
		count -= dropped
	}
	page := int32(len(m.Dialogs))
	if count < page {
		count = page
	}
	if exhausted || count <= page {
		return mtproto.MakeTLMessagesDialogs(&mtproto.Messages_Dialogs{
			Dialogs:  m.Dialogs,
			Messages: m.Messages,
			Chats:    m.Chats,
			Users:    m.Users,
		}).To_Messages_Dialogs()
	}
	return mtproto.MakeTLMessagesDialogsSlice(&mtproto.Messages_Dialogs{
		Dialogs:  m.Dialogs,
		Messages: m.Messages,
		Chats:    m.Chats,
		Users:    m.Users,
		Count:    count,
	}).To_Messages_Dialogs()
}

// FixedDialogs runs fix() and returns the dialogs that can actually be sent.
func (m *DialogsDataHelper) FixedDialogs() []*mtproto.Dialog {
	if m == nil {
		return nil
	}
	m.fix()
	return m.Dialogs
}

// ForDialogs is the same loaded messages, users and chats, restricted to
// one page of dialogs. fix() trims the related rows.
func (m *DialogsDataHelper) ForDialogs(dialogs []*mtproto.Dialog) *DialogsDataHelper {
	if m == nil {
		return &DialogsDataHelper{Dialogs: dialogs}
	}
	return &DialogsDataHelper{
		Dialogs:  dialogs,
		Messages: m.Messages,
		Chats:    m.Chats,
		Users:    m.Users,
	}
}

func (m *DialogsDataHelper) ToMessagesPeerDialogs(state *mtproto.Updates_State) *mtproto.Messages_PeerDialogs {
	// Same filter as getDialogs. A dialog with top_message 0, or whose top
	// message is not in this response, is not a chat-list row. iOS merges
	// messages.peerDialogs into the chat list when a chat is opened; that
	// empty row reloads the list and the scroll position jumps to the top.
	m.fix()
	return mtproto.MakeTLMessagesPeerDialogs(&mtproto.Messages_PeerDialogs{
		Dialogs:  m.Dialogs,
		Messages: m.Messages,
		Users:    m.Users,
		Chats:    m.Chats,
		State:    state,
	}).To_Messages_PeerDialogs()
}
