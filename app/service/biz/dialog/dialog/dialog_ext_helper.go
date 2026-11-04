// Copyright (c) 2021-present,  Teamgram Studio (https://teamgram.io).
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package dialog

import (
	"context"
	"math"
	"sync"

	"github.com/teamgram/proto/mtproto"

	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/mr"
)

type (
	DialogExtList []*DialogExt
)

func (m DialogExtList) Len() int {
	return len(m)
}

// Count is messages.dialogsSlice count: the total number of dialogs in this
// folder view, not the number of dialogs on the current page.
func (m DialogExtList) Count() int32 {
	if m.Len() > math.MaxInt32 {
		return math.MaxInt32
	}
	return int32(m.Len())
}

func (m DialogExtList) Swap(i, j int) {
	m[j], m[i] = m[i], m[j]
}

// Less is the ascending order used by sort.Reverse. The reversed list is the
// chat list: pinned dialogs keep upstream pin rank (Order descending), then
// unpinned dialogs by top-message date descending, peer type, peer id.
func (m DialogExtList) Less(i, j int) bool {
	return dialogListBefore(m[j], m[i])
}

func (m DialogExtList) ChannelIdList() (idList []int64) {
	for _, dlgExt := range m {
		peer := dlgExt.GetDialog().GetPeer()
		if mtproto.PeerIsChannel(peer) {
			idList = append(idList, peer.ChannelId)
		}
	}

	return
}

// GetDialogsByOffsetLimit returns the next page of a chat list already ordered
// by sort.Reverse (see Less).
//
// messages.getDialogs pagination is not an exact row lookup. offset_date,
// offset_id and offset_peer are the top message of the last dialog the client
// already has; the page is every dialog strictly older than that cursor.
// Pinned dialogs stay in upstream pin order: a pinned cursor continues by pin
// rank, and an unpinned cursor does not pull pinned dialogs back onto the page.
// A cursor that no longer matches a row still continues from the date and peer
// instead of returning an empty page (which makes clients reload from the top).
func (m DialogExtList) GetDialogsByOffsetLimit(offsetDate int32, offsetId int32, offsetPeer *mtproto.PeerUtil, limit int32) DialogExtList {
	if m.Len() == 0 || limit <= 0 {
		return emptyDialogPage(m)
	}
	if dialogsFirstPage(offsetDate, offsetId, offsetPeer) {
		return m.dialogPage(0, limit, nil)
	}
	cur, ok := m.dialogCursor(offsetDate, offsetId, offsetPeer)
	if !ok {
		return emptyDialogPage(m)
	}
	return m.dialogPage(m.firstStrictlyOlder(cur), limit, offsetPeer)
}

func emptyDialogPage(m DialogExtList) DialogExtList {
	if m == nil {
		return DialogExtList{}
	}
	return m[:0]
}

func dialogsFirstPage(offsetDate, offsetId int32, offsetPeer *mtproto.PeerUtil) bool {
	if offsetPeer != nil && !offsetPeer.IsEmpty() {
		return false
	}
	_, dateOK := meaningfulOffsetDate(offsetDate)
	idOK := offsetId > 0 && offsetId < math.MaxInt32
	return !dateOK && !idOK
}

// dialogCursor is the chat-list position of messages.getDialogs' offset.
// pinned cursors use the upstream pin rank. Other cursors use offset_date
// plus peer type and peer id, which is the same total order as Less.
type dialogCursor struct {
	pinned   bool
	order    int64
	date     int64
	peerType int32
	peerID   int64
}

func (m DialogExtList) dialogCursor(offsetDate, offsetId int32, offsetPeer *mtproto.PeerUtil) (dialogCursor, bool) {
	cur := dialogCursor{peerType: math.MaxInt32, peerID: math.MaxInt64}
	if offsetPeer != nil && !offsetPeer.IsEmpty() {
		cur.peerType = normalizeDialogPeerType(offsetPeer.PeerType)
		cur.peerID = offsetPeer.PeerId
		if i := m.indexOfPeer(offsetPeer); i >= 0 {
			return cursorFromDialog(m[i], offsetDate, cur), true
		}
	}
	if date, ok := meaningfulOffsetDate(offsetDate); ok {
		cur.date = date
		return cur, true
	}
	if offsetId > 0 && offsetId < math.MaxInt32 {
		if i := m.indexOfTopMessage(offsetId); i >= 0 {
			typ, id := dialogPeer(m[i])
			return cursorFromDialog(m[i], 0, dialogCursor{peerType: typ, peerID: id}), true
		}
	}
	return dialogCursor{}, false
}

func cursorFromDialog(d *DialogExt, offsetDate int32, located dialogCursor) dialogCursor {
	if dialogPinned(d) {
		located.pinned = true
		located.order = d.GetOrder()
		return located
	}
	if date, ok := meaningfulOffsetDate(offsetDate); ok {
		located.date = date
		return located
	}
	located.date = dialogTopDate(d)
	return located
}

func (m DialogExtList) firstStrictlyOlder(cur dialogCursor) int {
	for i, d := range m {
		if d != nil && dialogStrictlyOlder(d, cur) {
			return i
		}
	}
	return m.Len()
}

func (m DialogExtList) dialogPage(start int, limit int32, skip *mtproto.PeerUtil) DialogExtList {
	if start < 0 {
		start = 0
	}
	out := make(DialogExtList, 0, limit)
	for i := start; i < m.Len() && int32(len(out)) < limit; i++ {
		if m[i] == nil {
			continue
		}
		if skip != nil && !skip.IsEmpty() && sameDialogPeer(m[i], skip) {
			continue
		}
		out = append(out, m[i])
	}
	return out
}

// dialogListBefore reports whether a is above b in the chat list.
func dialogListBefore(a, b *DialogExt) bool {
	if a == nil || b == nil {
		return a != nil && b == nil
	}
	return dialogStrictlyOlder(b, cursorOf(a))
}

func cursorOf(d *DialogExt) dialogCursor {
	typ, id := dialogPeer(d)
	cur := dialogCursor{peerType: typ, peerID: id, date: dialogTopDate(d)}
	if dialogPinned(d) {
		cur.pinned = true
		cur.order = d.GetOrder()
	}
	return cur
}

// dialogStrictlyOlder reports whether d sorts below cur in the chat list.
func dialogStrictlyOlder(d *DialogExt, cur dialogCursor) bool {
	if dialogPinned(d) != cur.pinned {
		return cur.pinned
	}
	if cur.pinned {
		if d.GetOrder() != cur.order {
			return d.GetOrder() < cur.order
		}
	} else if dialogTopDate(d) != cur.date {
		return dialogTopDate(d) < cur.date
	}
	typ, id := dialogPeer(d)
	if typ != cur.peerType {
		return typ > cur.peerType
	}
	return id > cur.peerID
}

func meaningfulOffsetDate(offsetDate int32) (int64, bool) {
	if offsetDate <= 0 || offsetDate == math.MaxInt32 {
		return 0, false
	}
	return int64(offsetDate), true
}

func (m DialogExtList) indexOfPeer(peer *mtproto.PeerUtil) int {
	if peer == nil || peer.IsEmpty() {
		return -1
	}
	want := normalizeDialogPeerType(peer.PeerType)
	for i, d := range m {
		typ, id := dialogPeer(d)
		if typ == want && id == peer.PeerId {
			return i
		}
	}
	return -1
}

func (m DialogExtList) indexOfTopMessage(top int32) int {
	for i, d := range m {
		if d.GetDialog().GetTopMessage() == top {
			return i
		}
	}
	return -1
}

func sameDialogPeer(d *DialogExt, peer *mtproto.PeerUtil) bool {
	if peer == nil || peer.IsEmpty() {
		return false
	}
	typ, id := dialogPeer(d)
	return typ == normalizeDialogPeerType(peer.PeerType) && id == peer.PeerId
}

func dialogPinned(d *DialogExt) bool {
	return d.GetDialog().GetPinned()
}

func dialogTopDate(d *DialogExt) int64 {
	if d == nil {
		return 0
	}
	if d.Date != 0 {
		return d.Date
	}
	if !dialogPinned(d) {
		return d.Order
	}
	return 0
}

func dialogPeer(d *DialogExt) (int32, int64) {
	peer := d.GetDialog().GetPeer()
	if peer == nil {
		return mtproto.PEER_UNKNOWN, 0
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_peerChat:
		return mtproto.PEER_CHAT, peer.GetChatId()
	case mtproto.Predicate_peerChannel:
		return mtproto.PEER_CHANNEL, peer.GetChannelId()
	case mtproto.Predicate_peerUser:
		return mtproto.PEER_USER, peer.GetUserId()
	default:
		return mtproto.PEER_UNKNOWN, 0
	}
}

func normalizeDialogPeerType(peerType int32) int32 {
	if peerType == mtproto.PEER_SELF {
		return mtproto.PEER_USER
	}
	return peerType
}

func (m DialogExtList) DebugString() string {
	s, _ := jsonx.MarshalToString(m)
	return s
}

type TopMessageId struct {
	Peer       *mtproto.PeerUtil
	TopMessage int32
}

type PeerIdList struct {
	PeerType int
	IdList   []int64
}

func (m DialogExtList) DoGetMessagesDialogs(
	ctx context.Context,
	selfUserId int64,
	// cbNotifySettings func(ctx context.Context, selfUserId int64, peers ...mtproto.PeerUtil) []*mtproto.PeerNotifySettings,
	cbMsgF func(ctx context.Context, selfUserId int64, id ...TopMessageId) []*mtproto.Message,
	cbUserF func(ctx context.Context, selfUserId int64, id ...int64) []*mtproto.User,
	cbChatF func(ctx context.Context, selfUserId int64, id ...int64) []*mtproto.Chat,
	cbChannelF func(ctx context.Context, selfUserId int64, id ...int64) []*mtproto.Chat) *DialogsDataHelper {

	dialogsData := &DialogsDataHelper{
		Dialogs:  []*mtproto.Dialog{},
		Messages: []*mtproto.Message{},
		Chats:    []*mtproto.Chat{},
		Users:    []*mtproto.User{},
	}

	if len(m) == 0 {
		return dialogsData
	}

	var (
		idHelper = mtproto.NewIDListHelper(selfUserId)
		mu       sync.Mutex
	)

	mr.ForEach(
		func(source chan<- interface{}) {
			var (
				idList   []TopMessageId
				chIdList []TopMessageId
			)

			for _, dialogExt := range m {
				peer2 := mtproto.FromPeer(dialogExt.Dialog.Peer)
				idHelper.PickByPeer(dialogExt.Dialog.Peer)
				if peer2.IsChannel() {
					chIdList = append(chIdList, TopMessageId{Peer: peer2, TopMessage: dialogExt.Dialog.TopMessage})
				} else {
					idList = append(idList, TopMessageId{Peer: peer2, TopMessage: dialogExt.Dialog.TopMessage})
				}
			}
			if len(idList) > 0 {
				source <- idList
			}
			if len(chIdList) > 0 {
				source <- chIdList
			}
		},
		func(item interface{}) {
			idList := item.([]TopMessageId)
			mList := cbMsgF(ctx, selfUserId, idList...)
			for _, msg := range mList {
				mu.Lock()
				idHelper.PickByMessage(msg)
				dialogsData.Messages = append(dialogsData.Messages, msg)
				mu.Unlock()
			}
		})

	for _, dialogExt := range m {
		dialogsData.Dialogs = append(dialogsData.Dialogs, dialogExt.Dialog)
	}

	mr.ForEach(
		func(source chan<- interface{}) {
			idHelper.Visit(
				func(userIdList []int64) {
					if cbUserF != nil {
						source <- PeerIdList{mtproto.PEER_USER, userIdList}
					}
				},
				func(chatIdList []int64) {
					if cbChatF != nil {
						source <- PeerIdList{mtproto.PEER_CHAT, chatIdList}
					}
				},
				func(channelIdList []int64) {
					if cbChatF != nil {
						source <- PeerIdList{mtproto.PEER_CHANNEL, channelIdList}
					}
				})

		},
		func(item interface{}) {
			idList := item.(PeerIdList)
			switch idList.PeerType {
			case mtproto.PEER_USER:
				users := cbUserF(ctx, selfUserId, idList.IdList...)
				if len(users) > 0 {
					mu.Lock()
					dialogsData.Users = append(dialogsData.Users, users...)
					mu.Unlock()
				}
			case mtproto.PEER_CHAT:
				chats := cbChatF(ctx, selfUserId, idList.IdList...)
				if len(chats) > 0 {
					mu.Lock()
					dialogsData.Chats = append(dialogsData.Chats, chats...)
					mu.Unlock()
				}
			case mtproto.PEER_CHANNEL:
				chats := cbChannelF(ctx, selfUserId, idList.IdList...)
				if len(chats) > 0 {
					mu.Lock()
					dialogsData.Chats = append(dialogsData.Chats, chats...)
					mu.Unlock()
				}
			}
		})

	return dialogsData
}

func (m *DialogExt) HasDialog() bool {
	name := m.GetDialog().GetDraft().GetPredicateName()
	if name != mtproto.Predicate_draftMessage {
		return false
	}
	return true
}
