package engine

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/sfu"
	"google.golang.org/grpc/status"
)

const (
	maxChainBlock   = 64 << 10
	maxEncrypted    = 8 << 10
	maxCallText     = 4096
	maxChainPage    = 100
	chainSubDefault = 0
)

// gmsg is one phone.sendGroupCallMessage stored on a live call.
type gmsg struct {
	id       int32
	fromUser int64
	from     *mtproto.Peer
	admin    bool
	date     int32
	text     *mtproto.TextWithEntities
	paid     *int64
}

// confInvite is one phone.inviteConferenceCallParticipant, keyed by message id.
type confInvite struct {
	msgID          int32
	callID, access int64
	from, to       int64
	video          bool
	declined       bool
}

// ConferenceInvite is the result of inviting a user into a conference.
type ConferenceInvite struct {
	MsgID          int32
	From, To       int64
	CallID, Access int64
	Video          bool
	Updates        *mtproto.Updates
	Message        *mtproto.Message
}

// GroupText is phone.sendGroupCallMessage after the handler resolved peers.
type GroupText struct {
	UserID         int64
	CallID, Access int64
	RandomID       int64
	Text           *mtproto.TextWithEntities
	Allow          *int64
	SendAs         *mtproto.Peer
	Admin          bool
}

func chainUpdate(g *group, sub int32, blocks [][]byte, next int32) *mtproto.Update {
	return mtproto.MakeTLUpdateGroupCallChainBlocks(&mtproto.Update{
		Call_INPUTGROUPCALL: inputCall(g.id, g.access),
		SubChainId:          sub,
		Blocks:              blocks,
		NextOffset:          next,
	}).To_Update()
}

func (e *Engine) appendBlock(g *group, sub int32, block []byte) {
	if g.chains == nil {
		g.chains = map[int32][][]byte{}
	}
	g.chains[sub] = append(g.chains[sub], block)
}

func (g *group) live(user int64) *part {
	for _, p := range g.parts {
		if p != nil && !p.left && p.userID == user {
			return p
		}
	}
	return nil
}

func (e *Engine) conferenceByRandom(user int64, randomID int32) *group {
	if randomID == 0 {
		return nil
	}
	g := e.groups[e.byRandom[fmt.Sprintf("cf:%d:%d", user, randomID)]]
	if g == nil || g.discarded {
		return nil
	}
	return g
}

func (e *Engine) newConference(id, access, user, auth int64, randomID int32) *group {
	if id == 0 {
		id = newID()
	}
	if access == 0 {
		access = newID()
	}
	now := int32(e.now().Unix())
	g := &group{
		id: id, access: access, creator: user, creatorAuth: auth, date: now,
		joinMuted: false, messagesEnabled: true, conference: true, version: 1,
		invite: randHex(8), speakerInvite: randHex(8), randomID: randomID, rtmpKey: randHex(8),
		byKey: map[string]*part{}, subs: map[int64]bool{},
		chains: map[int32][][]byte{}, msgByID: map[int32]*gmsg{}, msgByRandom: map[string]int32{},
	}
	e.groups[g.id] = g
	if randomID != 0 {
		e.byRandom[fmt.Sprintf("cf:%d:%d", user, randomID)] = g.id
	}
	return g
}

// CreateConference is phone.createConferenceCall.
type CreateConference struct {
	UserID, Auth int64
	RandomID     int32
	Muted        bool
	VideoStopped bool
	Join         bool
	PublicKey    []byte
	Block        []byte
	Params       string
}

func (e *Engine) CreateConference(in CreateConference) (*mtproto.Updates, error) {
	if in.Join && len(in.PublicKey) != 32 {
		return nil, mtproto.ErrGroupcallInvalid
	}
	if len(in.Block) > maxChainBlock {
		return nil, mtproto.ErrGroupcallInvalid
	}
	e.mu.Lock()
	if g := e.conferenceByRandom(in.UserID, in.RandomID); g != nil {
		up := e.groupUpdates(in.UserID, g, nil, nil)
		e.mu.Unlock()
		return up, nil
	}
	cid, acc := newID(), newID()
	e.mu.Unlock()

	var (
		answer string
		ssrc   uint32
		groups []sfu.Group
	)
	key := partKey(mtproto.PEER_USER, in.UserID)
	if in.Join && in.Params != "" {
		var err error
		answer, ssrc, groups, err = e.answerOffer(cid, key, in.Params, fmt.Sprint(in.UserID))
		if err != nil {
			return nil, mtproto.ErrGroupcallInvalid
		}
	}
	e.mu.Lock()
	if g := e.conferenceByRandom(in.UserID, in.RandomID); g != nil {
		e.mu.Unlock()
		if e.media != nil {
			e.media.Leave(cid, key)
		}
		e.mu.Lock()
		up := e.groupUpdates(in.UserID, g, nil, nil)
		e.mu.Unlock()
		return up, nil
	}
	g := e.newConference(cid, acc, in.UserID, in.Auth, in.RandomID)
	var changed []*part
	var extra []*mtproto.Update
	if len(in.Block) > 0 {
		blk := append([]byte(nil), in.Block...)
		e.appendBlock(g, chainSubDefault, blk)
		extra = append(extra, chainUpdate(g, chainSubDefault, [][]byte{blk}, -1))
	}
	if in.Join {
		p := &part{
			userID: in.UserID, auth: in.Auth, peerType: mtproto.PEER_USER, peerID: in.UserID,
			date: int32(e.now().Unix()), source: int32(ssrc), muted: in.Muted, canUnmute: true,
			videoStopped: in.VideoStopped, video: groups,
		}
		g.parts = append(g.parts, p)
		g.byKey[partKey(p.peerType, p.peerID)] = p
		g.version++
		changed = []*part{p}
		if answer != "" {
			extra = append(extra, mtproto.MakeTLUpdateGroupCallConnection(&mtproto.Update{
				Params: dataJSON(answer),
			}).To_Update())
		}
	}
	up := e.fanout(g, in.UserID, in.Auth, changed, extra)
	e.mu.Unlock()
	return up, nil
}

// ConferenceFromCall is the legacy phone.createConferenceCall that returns phone.PhoneCall.
// It marks an active 1:1 call as conference-capable. The layer-232 method creates the room.
func (e *Engine) ConferenceFromCall(user, id, access, fingerprint int64) (*mtproto.Phone_PhoneCall, error) {
	e.mu.Lock()
	c, err := e.load(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if user != c.admin && user != c.participant {
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	}
	if c.phase != phaseActive || c.fingerprint == 0 || c.fingerprint != fingerprint {
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	}
	c.conference = true
	up := e.phoneResult(user, c, user == c.admin)
	notes := e.notesFor(c, user, c.admin, c.participant)
	e.mu.Unlock()
	e.deliver(notes)
	return up, nil
}

// DeleteConferenceParticipants is phone.deleteConferenceCallParticipants.
func (e *Engine) DeleteConferenceParticipants(user, id, access int64, onlyLeft, kick bool, ids []int64, block []byte) (*mtproto.Updates, error) {
	if len(block) > maxChainBlock {
		return nil, mtproto.ErrGroupcallInvalid
	}
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if g.live(user) == nil && user != g.creator {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallForbidden
	}
	var changed []*part
	for _, uid := range ids {
		var p *part
		for _, it := range g.parts {
			if it != nil && it.userID == uid {
				p = it
				break
			}
		}
		if p == nil || p.left {
			continue
		}
		if onlyLeft {
			continue
		}
		if user != g.creator && uid != user {
			e.mu.Unlock()
			return nil, mtproto.ErrChatAdminRequired
		}
		if kick || !onlyLeft {
			p.left = true
			changed = append(changed, p)
		}
	}
	var extra []*mtproto.Update
	if len(block) > 0 {
		blk := append([]byte(nil), block...)
		e.appendBlock(g, chainSubDefault, blk)
		extra = append(extra, chainUpdate(g, chainSubDefault, [][]byte{blk}, -1))
	}
	if len(changed) > 0 {
		g.version++
	}
	up := e.fanout(g, user, 0, changed, extra)
	e.mu.Unlock()
	return up, nil
}

// BroadcastBlock is phone.sendConferenceCallBroadcast.
func (e *Engine) BroadcastBlock(user, id, access int64, block []byte) (*mtproto.Updates, error) {
	if len(block) == 0 || len(block) > maxChainBlock {
		return nil, mtproto.ErrGroupcallInvalid
	}
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if g.live(user) == nil && user != g.creator {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	blk := append([]byte(nil), block...)
	e.appendBlock(g, chainSubDefault, blk)
	up := e.fanout(g, user, 0, nil, []*mtproto.Update{chainUpdate(g, chainSubDefault, [][]byte{blk}, -1)})
	e.mu.Unlock()
	return up, nil
}

// ChainBlocks is phone.getGroupCallChainBlocks.
func (e *Engine) ChainBlocks(user, id, access int64, sub, offset, limit int32) (*mtproto.Updates, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		return nil, err
	}
	blocks := g.chains[sub]
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > maxChainPage {
		limit = maxChainPage
	}
	if int(offset) > len(blocks) {
		offset = int32(len(blocks))
	}
	end := int(offset + limit)
	next := int32(end)
	if end >= len(blocks) {
		end = len(blocks)
		next = -1
	}
	page := make([][]byte, 0, end-int(offset))
	for _, b := range blocks[int(offset):end] {
		page = append(page, append([]byte(nil), b...))
	}
	ids := []int64{user, g.creator}
	return e.ups(user, ids, chainUpdate(g, sub, page, next)), nil
}

// InviteConference is phone.inviteConferenceCallParticipant.
func (e *Engine) InviteConference(user, auth, id, access, invitee int64, video bool) (*ConferenceInvite, error) {
	if invitee == 0 || invitee == user {
		return nil, mtproto.ErrUserIdInvalid
	}
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if !g.conference {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallInvalid
	}
	if g.live(user) == nil && user != g.creator {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	msgID := e.allocInviteID()
	var others []*mtproto.Peer
	for _, p := range g.parts {
		if p != nil && !p.left && p.userID != user && p.userID != invitee {
			others = append(others, peerOf(p.peerType, p.peerID))
		}
	}
	action := mtproto.MakeTLMessageActionConferenceCall(&mtproto.MessageAction{
		Active: true, Video: video, CallId: g.id, OtherParticipants: others,
	}).To_MessageAction()
	msg := mtproto.MakeTLMessageService(&mtproto.Message{
		Out: true, Id: msgID, FromId: mtproto.MakePeerUser(user), PeerId: mtproto.MakePeerUser(invitee),
		Date: int32(e.now().Unix()), Action: action,
	}).To_Message()
	e.invites[msgID] = &confInvite{
		msgID: msgID, callID: g.id, access: g.access, from: user, to: invitee, video: video,
	}
	up := e.groupUpdates(user, g, nil, []*mtproto.Update{
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: msg, Pts_INT32: 1, PtsCount: 1}).To_Update(),
	})
	note := e.groupUpdates(invitee, g, nil, []*mtproto.Update{
		mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
			Message_MESSAGE: mtproto.MakeTLMessageService(&mtproto.Message{
				Id: msgID, FromId: mtproto.MakePeerUser(user), PeerId: mtproto.MakePeerUser(invitee),
				Date: msg.Date, Action: action,
			}).To_Message(),
			Pts_INT32: 1, PtsCount: 1,
		}).To_Update(),
	})
	e.mu.Unlock()
	e.deliver([]Note{{UserID: invitee, Updates: note}, {UserID: user, Exclude: []int64{auth}, Updates: up}})
	return &ConferenceInvite{
		MsgID: msgID, From: user, To: invitee, CallID: id, Access: access, Video: video,
		Updates: up, Message: msg,
	}, nil
}

func (e *Engine) allocInviteID() int32 {
	if e.invites == nil {
		e.invites = map[int32]*confInvite{}
	}
	for {
		id := int32(newID() & 0x7fffffff)
		if id != 0 && e.invites[id] == nil {
			return id
		}
	}
}

// AliasInvite records the messenger's message id for the same invite.
func (e *Engine) AliasInvite(from, to int32) {
	if to == 0 || from == to {
		return
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if src := e.invites[from]; src != nil {
		e.invites[to] = src
	}
}

// DeclineConference is phone.declineConferenceCallInvite.
func (e *Engine) DeclineConference(user int64, msgID int32) (*mtproto.Updates, error) {
	e.mu.Lock()
	inv := e.invites[msgID]
	if inv == nil || inv.to != user {
		e.mu.Unlock()
		return nil, mtproto.ErrMsgIdInvalid
	}
	inv.declined = true
	action := mtproto.MakeTLMessageActionConferenceCall(&mtproto.MessageAction{
		Missed: true, Video: inv.video, CallId: inv.callID,
	}).To_MessageAction()
	msg := mtproto.MakeTLMessageService(&mtproto.Message{
		Id: inv.msgID, FromId: mtproto.MakePeerUser(inv.from), PeerId: mtproto.MakePeerUser(inv.to),
		Date: int32(e.now().Unix()), Action: action,
	}).To_Message()
	upd := mtproto.MakeTLUpdateNewMessage(&mtproto.Update{Message_MESSAGE: msg, Pts_INT32: 1, PtsCount: 1}).To_Update()
	var extra []*mtproto.Update
	extra = append(extra, upd)
	var up *mtproto.Updates
	if g := e.groups[inv.callID]; g != nil && g.access == inv.access && !g.discarded {
		up = e.groupUpdates(user, g, nil, extra)
		note := e.groupUpdates(inv.from, g, nil, extra)
		e.mu.Unlock()
		if inv.from != user {
			e.deliver([]Note{{UserID: inv.from, Updates: note}})
		}
		return up, nil
	}
	up = e.ups(user, []int64{inv.from, inv.to}, upd)
	e.mu.Unlock()
	if inv.from != user {
		e.deliver([]Note{{UserID: inv.from, Updates: e.ups(inv.from, []int64{inv.from, inv.to}, upd)}})
	}
	return up, nil
}

// SendGroupText is phone.sendGroupCallMessage. repeat is true when random_id was already used.
func (e *Engine) SendGroupText(in GroupText) (*mtproto.Updates, bool, error) {
	if in.Text == nil || in.Text.GetText() == "" {
		return nil, false, mtproto.ErrMessageEmpty
	}
	if utf8.RuneCountInString(in.Text.GetText()) > maxCallText {
		return nil, false, mtproto.ErrMessageTooLong
	}
	e.mu.Lock()
	g, err := e.loadGroup(in.CallID, in.Access)
	if err != nil {
		e.mu.Unlock()
		return nil, false, err
	}
	if !g.messagesEnabled {
		e.mu.Unlock()
		return nil, false, mtproto.ErrChatSendPlainForbidden
	}
	if g.live(in.UserID) == nil {
		e.mu.Unlock()
		return nil, false, mtproto.ErrGroupcallJoinMissing
	}
	if g.paidStars != nil && *g.paidStars > 0 {
		need := *g.paidStars
		if in.Allow == nil || *in.Allow < need {
			e.mu.Unlock()
			return nil, false, status.Error(mtproto.ErrForbidden, fmt.Sprintf("ALLOW_PAYMENT_REQUIRED_%d", need))
		}
	}
	from, err := sendAsPeer(g, in.UserID, in.SendAs, in.Admin)
	if err != nil {
		e.mu.Unlock()
		return nil, false, err
	}
	key := fmt.Sprintf("%d:%d", in.UserID, in.RandomID)
	if id, ok := g.msgByRandom[key]; ok {
		up := e.messageUpdates(in.UserID, g, g.msgByID[id])
		e.mu.Unlock()
		return up, true, nil
	}
	g.nextMsg++
	m := &gmsg{
		id: g.nextMsg, fromUser: in.UserID, from: from, admin: in.Admin || in.UserID == g.creator,
		date: int32(e.now().Unix()), text: in.Text,
	}
	if g.msgByID == nil {
		g.msgByID = map[int32]*gmsg{}
		g.msgByRandom = map[string]int32{}
	}
	g.msgs = append(g.msgs, m)
	g.msgByID[m.id] = m
	g.msgByRandom[key] = m.id
	upd := groupMessageUpdate(g, m)
	up := e.fanout(g, in.UserID, 0, nil, []*mtproto.Update{upd})
	e.mu.Unlock()
	return up, false, nil
}

func sendAsPeer(g *group, user int64, sendAs *mtproto.Peer, admin bool) (*mtproto.Peer, error) {
	if sendAs == nil {
		return mtproto.MakePeerUser(user), nil
	}
	switch sendAs.GetPredicateName() {
	case "", mtproto.Predicate_peerUser:
		if sendAs.GetUserId() != 0 && sendAs.GetUserId() != user {
			return nil, mtproto.ErrPeerIdInvalid
		}
		return mtproto.MakePeerUser(user), nil
	case mtproto.Predicate_peerChat:
		if g.peerType != mtproto.PEER_CHAT || g.peerID != sendAs.GetChatId() || (user != g.creator && !admin) {
			return nil, mtproto.ErrPeerIdInvalid
		}
		return peerOf(mtproto.PEER_CHAT, g.peerID), nil
	case mtproto.Predicate_peerChannel:
		if g.peerType != mtproto.PEER_CHANNEL || g.peerID != sendAs.GetChannelId() || (user != g.creator && !admin) {
			return nil, mtproto.ErrPeerIdInvalid
		}
		return peerOf(mtproto.PEER_CHANNEL, g.peerID), nil
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
}

func groupMessageUpdate(g *group, m *gmsg) *mtproto.Update {
	tl := &mtproto.GroupCallMessage{
		FromAdmin: m.admin, Id: m.id, FromId: m.from, Date: m.date, Message: m.text,
	}
	return mtproto.MakeTLUpdateGroupCallMessage(&mtproto.Update{
		Call_INPUTGROUPCALL:      inputCall(g.id, g.access),
		Message_GROUPCALLMESSAGE: mtproto.MakeTLGroupCallMessage(tl).To_GroupCallMessage(),
	}).To_Update()
}

func (e *Engine) messageUpdates(viewer int64, g *group, m *gmsg) *mtproto.Updates {
	if m == nil {
		return e.groupUpdates(viewer, g, nil, nil)
	}
	return e.ups(viewer, []int64{viewer, m.fromUser}, groupMessageUpdate(g, m))
}

// SendGroupEncrypted is phone.sendGroupCallEncryptedMessage.
func (e *Engine) SendGroupEncrypted(user, id, access int64, payload []byte) (*mtproto.Bool, error) {
	if len(payload) == 0 || len(payload) > maxEncrypted {
		return nil, mtproto.ErrEncryptedMessageInvalid
	}
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	p := g.live(user)
	if p == nil {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	upd := mtproto.MakeTLUpdateGroupCallEncryptedMessage(&mtproto.Update{
		Call_INPUTGROUPCALL: inputCall(g.id, g.access),
		FromId:              peerOf(p.peerType, p.peerID),
		EncryptedMessage:    append([]byte(nil), payload...),
	}).To_Update()
	e.fanout(g, user, p.auth, nil, []*mtproto.Update{upd})
	e.mu.Unlock()
	return mtproto.BoolTrue, nil
}

// DeleteGroupMessages is phone.deleteGroupCallMessages.
func (e *Engine) DeleteGroupMessages(user, id, access int64, admin bool, ids []int32) (*mtproto.Updates, error) {
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		return nil, err
	}
	if g.live(user) == nil && user != g.creator {
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	deleted := e.dropMessages(g, func(m *gmsg) bool {
		if !contains32(ids, m.id) {
			return false
		}
		return m.fromUser == user || user == g.creator || admin
	})
	return e.fanout(g, user, 0, nil, []*mtproto.Update{deleteMessagesUpdate(g, deleted)}), nil
}

// DeleteParticipantMessages is phone.deleteGroupCallParticipantMessages.
func (e *Engine) DeleteParticipantMessages(user, id, access int64, admin bool, peer *mtproto.Peer) (*mtproto.Updates, error) {
	if peer == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		return nil, err
	}
	if g.live(user) == nil && user != g.creator {
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	self := peer.GetPredicateName() == mtproto.Predicate_peerUser && (peer.GetUserId() == user || peer.GetUserId() == 0)
	if !self && user != g.creator && !admin {
		return nil, mtproto.ErrChatAdminRequired
	}
	deleted := e.dropMessages(g, func(m *gmsg) bool {
		return samePeer(m.from, peer) || (self && m.fromUser == user)
	})
	return e.fanout(g, user, 0, nil, []*mtproto.Update{deleteMessagesUpdate(g, deleted)}), nil
}

func (e *Engine) dropMessages(g *group, match func(*gmsg) bool) []int32 {
	var keep []*gmsg
	var ids []int32
	for _, m := range g.msgs {
		if m == nil || !match(m) {
			keep = append(keep, m)
			continue
		}
		ids = append(ids, m.id)
		delete(g.msgByID, m.id)
	}
	g.msgs = keep
	if ids == nil {
		ids = []int32{}
	}
	return ids
}

func deleteMessagesUpdate(g *group, ids []int32) *mtproto.Update {
	if ids == nil {
		ids = []int32{}
	}
	return mtproto.MakeTLUpdateDeleteGroupCallMessages(&mtproto.Update{
		Call_INPUTGROUPCALL: inputCall(g.id, g.access),
		Messages:            ids,
	}).To_Update()
}

func contains32(ids []int32, id int32) bool {
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

func samePeer(a, b *mtproto.Peer) bool {
	if a == nil || b == nil {
		return false
	}
	return a.GetPredicateName() == b.GetPredicateName() && a.GetUserId() == b.GetUserId() && a.GetChatId() == b.GetChatId() && a.GetChannelId() == b.GetChannelId()
}

// GroupStars is phone.getGroupCallStars. Stars are not debited; the total is what messages recorded.
func (e *Engine) GroupStars(user, id, access int64) (*mtproto.Phone_GroupCallStars, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		return nil, err
	}
	var total int64
	type donor struct {
		user  int64
		stars int64
	}
	by := map[int64]int64{}
	for _, m := range g.msgs {
		if m == nil || m.paid == nil || *m.paid == 0 {
			continue
		}
		total += *m.paid
		by[m.fromUser] += *m.paid
	}
	var top []*mtproto.GroupCallDonor
	for uid, stars := range by {
		top = append(top, mtproto.MakeTLGroupCallDonor(&mtproto.GroupCallDonor{
			My: uid == user, PeerId: mtproto.MakePeerUser(uid), Stars: stars, Top: true,
		}).To_GroupCallDonor())
	}
	if top == nil {
		top = []*mtproto.GroupCallDonor{}
	}
	return mtproto.MakeTLPhoneGroupCallStars(&mtproto.Phone_GroupCallStars{
		TotalStars: total, TopDonors: top, Chats: []*mtproto.Chat{}, Users: []*mtproto.User{},
	}).To_Phone_GroupCallStars(), nil
}

// SaveDefaultSendAs stores the peer used as the default speaker in this call.
func (e *Engine) SaveDefaultSendAs(user, id, access int64, as *mtproto.Peer) (*mtproto.Bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		return nil, err
	}
	if g.live(user) == nil && user != g.creator {
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	if _, err := sendAsPeer(g, user, as, user == g.creator); err != nil {
		return nil, err
	}
	g.defaultSendAs = as
	return mtproto.BoolTrue, nil
}

// SaveJoinAs remembers phone.saveDefaultGroupCallJoinAs for a chat.
func (e *Engine) SaveJoinAs(user int64, peerType int32, peerID int64, as *mtproto.Peer) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.joinAs == nil {
		e.joinAs = map[string]*mtproto.Peer{}
	}
	e.joinAs[fmt.Sprintf("%d:%d:%d", user, peerType, peerID)] = as
}

// JoinAs returns the saved default join-as peer for a chat.
func (e *Engine) JoinAs(user int64, peerType int32, peerID int64) *mtproto.Peer {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.joinAs == nil {
		return nil
	}
	return e.joinAs[fmt.Sprintf("%d:%d:%d", user, peerType, peerID)]
}

// ResolveCall accepts inputGroupCall, inputGroupCallSlug, and inputGroupCallInviteMessage.
func (e *Engine) ResolveCall(in *mtproto.InputGroupCall) (int64, int64, error) {
	if in == nil {
		return 0, 0, mtproto.ErrGroupcallInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputGroupCallSlug:
		return e.lookupSlug(in.GetSlug())
	case mtproto.Predicate_inputGroupCallInviteMessage:
		return e.lookupInvite(in.GetMsgId())
	default:
		if in.GetId() == 0 && in.GetSlug() != "" {
			return e.lookupSlug(in.GetSlug())
		}
		if in.GetId() == 0 && in.GetMsgId() != 0 {
			return e.lookupInvite(in.GetMsgId())
		}
		if in.GetId() == 0 {
			return 0, 0, mtproto.ErrGroupcallInvalid
		}
		return in.GetId(), in.GetAccessHash(), nil
	}
}

func (e *Engine) lookupSlug(slug string) (int64, int64, error) {
	slug = strings.TrimSpace(slug)
	if e.cfg.CallLinkPrefix != "" {
		slug = strings.TrimPrefix(slug, e.cfg.CallLinkPrefix)
	}
	slug = strings.Trim(slug, "/")
	if i := strings.LastIndex(slug, "/"); i >= 0 {
		slug = slug[i+1:]
	}
	if slug == "" {
		return 0, 0, mtproto.ErrInviteHashInvalid
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	for _, g := range e.groups {
		if g == nil || g.discarded {
			continue
		}
		if g.invite == slug || g.speakerInvite == slug {
			return g.id, g.access, nil
		}
	}
	return 0, 0, mtproto.ErrInviteHashInvalid
}

func (e *Engine) lookupInvite(msgID int32) (int64, int64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	inv := e.invites[msgID]
	if inv == nil {
		return 0, 0, mtproto.ErrMsgIdInvalid
	}
	return inv.callID, inv.access, nil
}
