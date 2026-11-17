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
	m.fixRows(false)
}

// fixPeerDialogs is messages.getPeerDialogs. A requested peer with no dialog
// row still has a dialog, top_message 0 and no messages. Android opens a new
// 1:1 with load type LOAD_FROM_UNREAD and, when that reply's dialogs vector
// is empty, never calls messages.getHistory. The chat stays on the centre
// spinner. The chat list (fix) still drops top_message 0 so an empty row is
// not a chat-list entry.
func (m *DialogsDataHelper) fixPeerDialogs() {
	m.fixRows(true)
}

func (m *DialogsDataHelper) fixRows(keepEmpty bool) {
	var (
		dialogs  = make([]*mtproto.Dialog, 0, len(m.Dialogs))
		messages = make([]*mtproto.Message, 0, len(m.Messages))
		chats    = make([]*mtproto.Chat, 0, len(m.Chats))
		users    = make([]*mtproto.User, 0, len(m.Users))
	)

	for _, dlg := range m.Dialogs {
		if dlg == nil {
			continue
		}
		var (
			topMessage *mtproto.Message
		)

		if dlg.TopMessage == 0 {
			// getPeerDialogs keeps an empty row so Android can open a new
			// 1:1. The chat list does not: a private chat with no messages
			// is gone until the next message. A basic group or channel the
			// user still belongs to stays, with no message. That is what
			// official getDialogs returns after the last post is deleted.
			if keepEmpty {
				dialogs = append(dialogs, dlg)
				appendDialogPeer(dlg, m.Users, m.Chats, &users, &chats)
			} else if membershipDialog(dlg) && appendDialogPeer(dlg, m.Users, m.Chats, &users, &chats) {
				dialogs = append(dialogs, dlg)
			}
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

func membershipDialog(dlg *mtproto.Dialog) bool {
	if dlg == nil {
		return false
	}
	switch mtproto.FromPeer(dlg.Peer).PeerType {
	case mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		return true
	default:
		return false
	}
}

func appendDialogPeer(dlg *mtproto.Dialog, allUsers []*mtproto.User, allChats []*mtproto.Chat, users *[]*mtproto.User, chats *[]*mtproto.Chat) bool {
	if dlg == nil || dlg.Peer == nil {
		return false
	}
	p := mtproto.FromPeer(dlg.Peer)
	switch p.PeerType {
	case mtproto.PEER_USER, mtproto.PEER_SELF:
		for _, v := range allUsers {
			if v != nil && v.Id == p.PeerId {
				*users = append(*users, v)
				return true
			}
		}
	case mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		for _, v := range allChats {
			if v != nil && v.Id == p.PeerId {
				*chats = append(*chats, v)
				return true
			}
		}
	}
	return false
}

// ToMessagesDialogs builds the messages.getDialogs reply.
// count is the number of dialogs that can be returned, measured after fix().
// fullList is true only for the first page with no offset when that count
// fits in limit. The reply is then messages.dialogs. Every offset request,
// including an empty page, is messages.dialogsSlice with that total, so the
// client does not replace a non-empty folder with an empty list.
func (m *DialogsDataHelper) ToMessagesDialogs(count int32, fullList bool) *mtproto.Messages_Dialogs {
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
	if fullList {
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
	// A positive top_message still needs its message, same as the chat list.
	// top_message 0 is the official empty dialog for a peer that has no row
	// yet. Dropping it makes Android's first open skip getHistory.
	m.fixPeerDialogs()
	return mtproto.MakeTLMessagesPeerDialogs(&mtproto.Messages_PeerDialogs{
		Dialogs:  m.Dialogs,
		Messages: m.Messages,
		Users:    m.Users,
		Chats:    m.Chats,
		State:    state,
	}).To_Messages_PeerDialogs()
}
