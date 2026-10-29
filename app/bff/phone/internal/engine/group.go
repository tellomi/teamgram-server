package engine

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/sfu"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type part struct {
	userID             int64
	auth               int64
	peerType           int32
	peerID             int64
	date               int32
	source             int32
	muted              bool
	canUnmute          bool
	left               bool
	volume             int32
	hasVolume          bool
	volumeByAdmin      bool
	raiseHand          int64
	videoStopped       bool
	videoPaused        bool
	presentationPaused bool
	video              []sfu.Group
	presentation       []sfu.Group
	selfUnmuteByInvite bool
}

type group struct {
	id, access      int64
	peerType        int32
	peerID          int64
	creator         int64
	creatorAuth     int64
	title           string
	date            int32
	joinMuted       bool
	messagesEnabled bool
	rtmp            bool
	recordStart     int32
	recordVideo     bool
	schedule        int32
	version         int32
	invite          string
	speakerInvite   string
	discarded       bool
	parts           []*part
	byKey           map[string]*part
	subs            map[int64]bool
	blocks          [][]byte
	randomID        int32
	rtmpKey         string
	paidStars       *int64
}

func canAdmin(g *group, user int64, manage bool) bool {
	return g != nil && (user == g.creator || manage)
}

func partKey(peerType int32, id int64) string {
	return fmt.Sprintf("%d:%d", peerType, id)
}

func (g *group) binding() Bind {
	n := 0
	for _, p := range g.parts {
		if p != nil && !p.left {
			n++
		}
	}
	return Bind{
		PeerType: g.peerType, PeerID: g.peerID, CallID: g.id, Access: g.access,
		Active: !g.discarded, NotEmpty: n > 0,
	}
}

func (g *group) liveCount() int32 {
	var n int32
	for _, p := range g.parts {
		if p != nil && !p.left {
			n++
		}
	}
	return n
}

// GroupPeer returns the chat or channel that owns the call.
func (e *Engine) GroupPeer(id, access int64) (int32, int64, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		return 0, 0, err
	}
	return g.peerType, g.peerID, nil
}

func (e *Engine) loadGroup(id, access int64) (*group, error) {
	g := e.groups[id]
	if g == nil || g.access != access {
		return nil, mtproto.ErrGroupcallInvalid
	}
	if g.discarded {
		return nil, mtproto.ErrGroupcallAlreadyDiscarded
	}
	return g, nil
}

// CreateGroup is phone.createGroupCall. manage must already be checked.
type CreateGroup struct {
	UserID, Auth int64
	PeerType     int32
	PeerID       int64
	RandomID     int32
	Title        string
	Schedule     int32
	RTMP         bool
}

func (e *Engine) CreateGroup(in CreateGroup) (*mtproto.Updates, error) {
	if in.PeerType != mtproto.PEER_CHAT && in.PeerType != mtproto.PEER_CHANNEL {
		return nil, mtproto.ErrPeerIdInvalid
	}
	e.mu.Lock()
	rkey := fmt.Sprintf("g:%d:%d:%d", in.PeerType, in.PeerID, in.RandomID)
	if id, ok := e.byRandom[rkey]; ok {
		if g := e.groups[id]; g != nil && !g.discarded {
			up := e.groupUpdates(in.UserID, g, nil, nil)
			e.mu.Unlock()
			return up, nil
		}
	}
	pkey := partKey(in.PeerType, in.PeerID)
	if id, ok := e.byPeer[pkey]; ok {
		if g := e.groups[id]; g != nil && !g.discarded {
			e.mu.Unlock()
			return nil, mtproto.ErrGroupcallAlreadyStarted
		}
	}
	now := int32(e.now().Unix())
	g := &group{
		id: newID(), access: newID(), peerType: in.PeerType, peerID: in.PeerID,
		creator: in.UserID, creatorAuth: in.Auth, title: in.Title, date: now,
		joinMuted: true, messagesEnabled: true, rtmp: in.RTMP, schedule: in.Schedule,
		version: 1, invite: randHex(8), speakerInvite: randHex(8),
		byKey: map[string]*part{}, subs: map[int64]bool{}, randomID: in.RandomID,
		rtmpKey: randHex(8),
	}
	e.groups[g.id] = g
	e.byPeer[pkey] = g.id
	e.byRandom[rkey] = g.id
	up := e.fanout(g, in.UserID, in.Auth, nil, nil)
	ev := &GroupEnd{
		From: in.UserID, PeerType: in.PeerType, PeerID: in.PeerID, RandomID: newID(),
		Action: mtproto.MakeMessageActionGroupCall(inputCall(g.id, g.access), 0),
	}
	if in.Schedule > 0 {
		ev.Action = mtproto.MakeMessageActionGroupCallScheduled(inputCall(g.id, g.access), in.Schedule)
	}
	bind := g.binding()
	e.mu.Unlock()
	e.finishGroup(ev, &bind)
	return up, nil
}

func (e *Engine) fanout(g *group, actor, actorAuth int64, changed []*part, extra []*mtproto.Update) *mtproto.Updates {
	ids := e.recipients(g)
	var actorUp *mtproto.Updates
	var notes []Note
	for _, uid := range ids {
		up := e.groupUpdates(uid, g, changed, extra)
		if uid == actor {
			actorUp = up
			if actorAuth != 0 {
				notes = append(notes, Note{UserID: uid, Exclude: []int64{actorAuth}, Updates: up})
				continue
			}
		}
		notes = append(notes, Note{UserID: uid, Updates: up})
	}
	if actorUp == nil {
		actorUp = e.groupUpdates(actor, g, changed, extra)
	}
	e.deliver(notes)
	return actorUp
}

func (e *Engine) recipients(g *group) []int64 {
	seen := map[int64]bool{}
	var ids []int64
	add := func(id int64) {
		if id == 0 || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}
	add(g.creator)
	for _, p := range g.parts {
		if p != nil && !p.left {
			add(p.userID)
		}
	}
	if e.members != nil {
		for _, id := range e.members(g.peerType, g.peerID) {
			add(id)
		}
	}
	return ids
}

func (e *Engine) groupUpdates(viewer int64, g *group, changed []*part, extra []*mtproto.Update) *mtproto.Updates {
	var ups []*mtproto.Update
	ups = append(ups, mtproto.MakeTLUpdateGroupCall(&mtproto.Update{
		Peer_PEER:      peerOf(g.peerType, g.peerID),
		Call_GROUPCALL: e.groupTL(g, viewer),
	}).To_Update())
	if len(changed) > 0 {
		var parts []*mtproto.GroupCallParticipant
		for _, p := range changed {
			parts = append(parts, e.partTL(p, viewer, true))
		}
		ups = append(ups, mtproto.MakeTLUpdateGroupCallParticipants(&mtproto.Update{
			Call_INPUTGROUPCALL:                     inputCall(g.id, g.access),
			Participants_VECTORGROUPCALLPARTICIPANT: parts,
			Version:                                 g.version,
		}).To_Update())
	}
	ups = append(ups, extra...)
	userIDs := []int64{g.creator}
	for _, p := range g.parts {
		if p != nil {
			userIDs = append(userIDs, p.userID)
		}
	}
	up := e.ups(viewer, userIDs, ups...)
	if e.chats != nil {
		if chats := e.chats(viewer, g.peerType, g.peerID); len(chats) > 0 {
			up.Chats = chats
		}
	}
	if up.Chats == nil {
		up.Chats = []*mtproto.Chat{}
	}
	return up
}

func (e *Engine) groupTL(g *group, viewer int64) *mtproto.GroupCall {
	if g.discarded {
		dur := int32(e.now().Unix()) - g.date
		if dur < 0 {
			dur = 0
		}
		return mtproto.MakeTLGroupCallDiscarded(&mtproto.GroupCall{
			Id: g.id, AccessHash: g.access, Duration: dur,
		}).To_GroupCall()
	}
	video := int32(0)
	for _, p := range g.parts {
		if p != nil && !p.left && !p.videoStopped && len(p.video) > 0 {
			video++
		}
	}
	gc := &mtproto.GroupCall{
		JoinMuted: g.joinMuted, CanChangeJoinMuted: viewer == g.creator,
		CanStartVideo: true, RecordVideoActive: g.recordStart != 0 && g.recordVideo,
		RtmpStream: g.rtmp, Creator: viewer == g.creator,
		MessagesEnabled: g.messagesEnabled, CanChangeMessagesEnabled: viewer == g.creator,
		ParticipantsCount: g.liveCount(), UnmutedVideoLimit: 30, Version: g.version,
		ScheduleStartSubscribed: g.subs[viewer],
	}
	gc.Id = g.id
	gc.AccessHash = g.access
	if g.title != "" {
		gc.Title = wrapperspb.String(g.title)
	}
	if g.recordStart != 0 {
		gc.RecordStartDate = wrapperspb.Int32(g.recordStart)
	}
	if g.schedule != 0 {
		gc.ScheduleDate = wrapperspb.Int32(g.schedule)
	}
	if video > 0 {
		gc.UnmutedVideoCount = wrapperspb.Int32(video)
	}
	if g.invite != "" {
		gc.InviteLink = wrapperspb.String(e.cfg.CallLinkPrefix + g.invite)
	}
	if g.paidStars != nil {
		gc.SendPaidMessagesStars = wrapperspb.Int64(*g.paidStars)
	}
	return mtproto.MakeTLGroupCall(gc).To_GroupCall()
}

func (e *Engine) partTL(p *part, viewer int64, just bool) *mtproto.GroupCallParticipant {
	tl := &mtproto.GroupCallParticipant{
		Muted: p.muted, Left: p.left, CanSelfUnmute: p.canUnmute,
		JustJoined: just && !p.left, Versioned: true, Self: viewer == p.userID,
		VideoJoined: len(p.video) > 0 && !p.videoStopped,
		Peer:        peerOf(p.peerType, p.peerID), Date: p.date, Source: p.source,
		VolumeByAdmin: p.volumeByAdmin,
	}
	if !p.left {
		tl.ActiveDate = wrapperspb.Int32(p.date)
	}
	if p.hasVolume {
		tl.Volume = wrapperspb.Int32(p.volume)
	}
	if p.raiseHand != 0 {
		tl.RaiseHandRating = wrapperspb.Int64(p.raiseHand)
	}
	if len(p.video) > 0 && !p.videoStopped {
		tl.Video = videoTL(strconv.FormatInt(p.userID, 10), p.source, p.video, p.videoPaused)
	}
	if len(p.presentation) > 0 {
		tl.Presentation = videoTL(strconv.FormatInt(p.userID, 10)+":screen", p.source, p.presentation, p.presentationPaused)
	}
	return mtproto.MakeTLGroupCallParticipant(tl).To_GroupCallParticipant()
}

func videoTL(endpoint string, audio int32, groups []sfu.Group, paused bool) *mtproto.GroupCallParticipantVideo {
	var src []*mtproto.GroupCallParticipantVideoSourceGroup
	for _, g := range groups {
		src = append(src, mtproto.MakeTLGroupCallParticipantVideoSourceGroup(&mtproto.GroupCallParticipantVideoSourceGroup{
			Semantics: g.Semantics, Sources: append([]int32(nil), g.Sources...),
		}).To_GroupCallParticipantVideoSourceGroup())
	}
	v := &mtproto.GroupCallParticipantVideo{
		Paused: paused, Endpoint: endpoint, SourceGroups: src,
		AudioSource: wrapperspb.Int32(audio),
	}
	return mtproto.MakeTLGroupCallParticipantVideo(v).To_GroupCallParticipantVideo()
}

// JoinGroup is phone.joinGroupCall.
type JoinGroup struct {
	UserID, Auth   int64
	CallID, Access int64
	Muted          bool
	VideoStopped   bool
	PeerType       int32
	PeerID         int64
	InviteHash     string
	Params         string
	Block          []byte
	Member         bool
	Manage         bool
}

func (e *Engine) JoinGroup(in JoinGroup) (*mtproto.Updates, error) {
	if in.PeerType == 0 {
		in.PeerType = mtproto.PEER_USER
		in.PeerID = in.UserID
	}
	answer, ssrc, groups, err := e.answerOffer(in.CallID, partKey(in.PeerType, in.PeerID), in.Params, strconv.FormatInt(in.PeerID, 10))
	if err != nil {
		return nil, mtproto.ErrGroupcallInvalid
	}
	e.mu.Lock()
	g, err := e.loadGroup(in.CallID, in.Access)
	if err != nil {
		e.mu.Unlock()
		if e.media != nil {
			e.media.Leave(in.CallID, partKey(in.PeerType, in.PeerID))
		}
		return nil, err
	}
	if g.schedule != 0 && int32(e.now().Unix()) < g.schedule && in.UserID != g.creator {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallInvalid
	}
	speaker := in.InviteHash != "" && in.InviteHash == g.speakerInvite
	listener := in.InviteHash != "" && in.InviteHash == g.invite
	if !in.Member && !speaker && !listener && in.UserID != g.creator {
		e.mu.Unlock()
		return nil, mtproto.ErrInviteHashInvalid
	}
	if ssrc != 0 {
		for _, p := range g.parts {
			if p != nil && !p.left && p.source == int32(ssrc) && partKey(p.peerType, p.peerID) != partKey(in.PeerType, in.PeerID) {
				e.mu.Unlock()
				return nil, mtproto.ErrGroupcallSsrcDuplicateMuch
			}
		}
	}
	can := in.Manage || speaker || in.UserID == g.creator || !g.joinMuted
	muted := in.Muted || (g.joinMuted && !can)
	p := g.byKey[partKey(in.PeerType, in.PeerID)]
	if p == nil {
		p = &part{userID: in.UserID, peerType: in.PeerType, peerID: in.PeerID}
		g.parts = append(g.parts, p)
		g.byKey[partKey(in.PeerType, in.PeerID)] = p
	}
	p.userID = in.UserID
	p.auth = in.Auth
	p.date = int32(e.now().Unix())
	p.source = int32(ssrc)
	p.muted = muted
	p.canUnmute = can
	p.left = false
	p.videoStopped = in.VideoStopped
	p.video = groups
	g.version++
	var extra []*mtproto.Update
	extra = append(extra, mtproto.MakeTLUpdateGroupCallConnection(&mtproto.Update{
		Params: dataJSON(answer),
	}).To_Update())
	if len(in.Block) > 0 {
		g.blocks = append(g.blocks, append([]byte(nil), in.Block...))
		extra = append(extra, mtproto.MakeTLUpdateGroupCallChainBlocks(&mtproto.Update{
			Call_INPUTGROUPCALL: inputCall(g.id, g.access),
			Blocks:              [][]byte{append([]byte(nil), in.Block...)},
			NextOffset:          -1,
		}).To_Update())
	}
	up := e.fanout(g, in.UserID, in.Auth, []*part{p}, extra)
	bind := g.binding()
	e.mu.Unlock()
	e.finishGroup(nil, &bind)
	return up, nil
}

func (e *Engine) LeaveGroup(user, id, access int64, source int32) (*mtproto.Updates, error) {
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	var found *part
	for _, p := range g.parts {
		if p != nil && !p.left && p.userID == user && (source == 0 || p.source == source) {
			found = p
			break
		}
	}
	if found == nil {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	found.left = true
	g.version++
	if e.media != nil {
		e.media.Leave(g.id, partKey(found.peerType, found.peerID))
	}
	up := e.fanout(g, user, found.auth, []*part{found}, nil)
	bind := g.binding()
	e.mu.Unlock()
	e.finishGroup(nil, &bind)
	return up, nil
}

func (e *Engine) DiscardGroup(user, id, access int64, manage bool) (*mtproto.Updates, error) {
	e.mu.Lock()
	g := e.groups[id]
	if g == nil || g.access != access {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallInvalid
	}
	if g.discarded {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallAlreadyDiscarded
	}
	if !canAdmin(g, user, manage) {
		e.mu.Unlock()
		return nil, mtproto.ErrChatAdminRequired
	}
	g.discarded = true
	g.version++
	dur := int32(e.now().Unix()) - g.date
	if dur < 0 {
		dur = 0
	}
	delete(e.byPeer, partKey(g.peerType, g.peerID))
	if e.media != nil {
		for _, p := range g.parts {
			e.media.Leave(g.id, partKey(p.peerType, p.peerID))
		}
	}
	up := e.fanout(g, user, g.creatorAuth, nil, nil)
	ev := &GroupEnd{
		From: user, PeerType: g.peerType, PeerID: g.peerID, RandomID: newID(),
		Action: mtproto.MakeMessageActionGroupCall(inputCall(g.id, g.access), dur),
	}
	bind := g.binding()
	e.mu.Unlock()
	e.finishGroup(ev, &bind)
	return up, nil
}

func (e *Engine) GetGroup(viewer, id, access int64, limit int32) (*mtproto.Phone_GroupCall, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		return nil, err
	}
	return e.phoneGroup(viewer, g, nil, limit), nil
}

func (e *Engine) phoneGroup(viewer int64, g *group, only []*part, limit int32) *mtproto.Phone_GroupCall {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	list := only
	if list == nil {
		for _, p := range g.parts {
			if p != nil && !p.left {
				list = append(list, p)
			}
		}
	}
	if int32(len(list)) > limit {
		list = list[:limit]
	}
	var parts []*mtproto.GroupCallParticipant
	var ids []int64
	for _, p := range list {
		parts = append(parts, e.partTL(p, viewer, false))
		ids = append(ids, p.userID)
	}
	if parts == nil {
		parts = []*mtproto.GroupCallParticipant{}
	}
	chats := []*mtproto.Chat{}
	if e.chats != nil {
		if c := e.chats(viewer, g.peerType, g.peerID); c != nil {
			chats = c
		}
	}
	users := []*mtproto.User{}
	if e.users != nil && len(ids) > 0 {
		if u := e.users(viewer, ids); u != nil {
			users = u
		}
	}
	return mtproto.MakeTLPhoneGroupCall(&mtproto.Phone_GroupCall{
		Call: e.groupTL(g, viewer), Participants: parts, Chats: chats, Users: users,
	}).To_Phone_GroupCall()
}

func (e *Engine) Participants(viewer, id, access int64, sources []int32, peers []string, offset string, limit int32) (*mtproto.Phone_GroupParticipants, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	start := 0
	if offset != "" {
		fmt.Sscanf(offset, "%d", &start)
	}
	var all []*part
	for _, p := range g.parts {
		if p == nil {
			continue
		}
		if len(sources) > 0 {
			ok := false
			for _, s := range sources {
				if p.source == s {
					ok = true
				}
			}
			if !ok {
				continue
			}
		}
		if len(peers) > 0 {
			ok := false
			key := partKey(p.peerType, p.peerID)
			for _, s := range peers {
				if s == key {
					ok = true
				}
			}
			if !ok {
				continue
			}
		}
		all = append(all, p)
	}
	next := ""
	if start < len(all) {
		all = all[start:]
	} else {
		all = nil
	}
	if int32(len(all)) > limit {
		next = strconv.Itoa(start + int(limit))
		all = all[:limit]
	}
	var parts []*mtproto.GroupCallParticipant
	var ids []int64
	for _, p := range all {
		parts = append(parts, e.partTL(p, viewer, false))
		ids = append(ids, p.userID)
	}
	if parts == nil {
		parts = []*mtproto.GroupCallParticipant{}
	}
	users := []*mtproto.User{}
	if e.users != nil && len(ids) > 0 {
		if u := e.users(viewer, ids); u != nil {
			users = u
		}
	}
	return mtproto.MakeTLPhoneGroupParticipants(&mtproto.Phone_GroupParticipants{
		Count: int32(len(g.parts)), Participants: parts, NextOffset: next,
		Chats: []*mtproto.Chat{}, Users: users, Version: g.version,
	}).To_Phone_GroupParticipants(), nil
}

func (e *Engine) CheckSources(id, access int64, sources []int32) ([]int32, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		return nil, err
	}
	var out []int32
	for _, s := range sources {
		for _, p := range g.parts {
			if p != nil && !p.left && p.source == s {
				out = append(out, s)
			}
		}
	}
	if out == nil {
		out = []int32{}
	}
	return out, nil
}

// EditPart is phone.editGroupCallParticipant.
type EditPart struct {
	UserID, CallID, Access int64
	PeerType               int32
	PeerID                 int64
	Manage                 bool
	Muted                  *bool
	Volume                 *int32
	Raise                  *bool
	VideoStopped           *bool
	VideoPaused            *bool
	PresentationPaused     *bool
}

func (e *Engine) EditParticipant(in EditPart) (*mtproto.Updates, error) {
	e.mu.Lock()
	g, err := e.loadGroup(in.CallID, in.Access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	p := g.byKey[partKey(in.PeerType, in.PeerID)]
	if p == nil || p.left {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	self := p.userID == in.UserID
	if !self && !in.Manage && in.UserID != g.creator {
		e.mu.Unlock()
		return nil, mtproto.ErrChatAdminRequired
	}
	if in.Muted != nil {
		if !self && *in.Muted == false && !in.Manage && in.UserID != g.creator {
			e.mu.Unlock()
			return nil, mtproto.ErrChatAdminRequired
		}
		if self && !*in.Muted && !p.canUnmute {
			e.mu.Unlock()
			return nil, mtproto.ErrChatAdminRequired
		}
		p.muted = *in.Muted
		if !self {
			p.canUnmute = !*in.Muted
		}
	}
	if in.Volume != nil {
		p.volume = *in.Volume
		p.hasVolume = true
		p.volumeByAdmin = !self
	}
	if in.Raise != nil {
		if *in.Raise {
			p.raiseHand = e.now().UnixMilli()
		} else {
			p.raiseHand = 0
		}
	}
	if in.VideoStopped != nil {
		p.videoStopped = *in.VideoStopped
	}
	if in.VideoPaused != nil {
		p.videoPaused = *in.VideoPaused
	}
	if in.PresentationPaused != nil {
		p.presentationPaused = *in.PresentationPaused
	}
	g.version++
	up := e.fanout(g, in.UserID, 0, []*part{p}, nil)
	e.mu.Unlock()
	return up, nil
}

func (e *Engine) ToggleSettings(user, id, access int64, manage, reset bool, joinMuted, messages *bool, stars *int64) (*mtproto.Updates, error) {
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if !canAdmin(g, user, manage) {
		e.mu.Unlock()
		return nil, mtproto.ErrChatAdminRequired
	}
	if joinMuted != nil {
		g.joinMuted = *joinMuted
	}
	if messages != nil {
		g.messagesEnabled = *messages
	}
	if reset {
		g.invite = randHex(8)
		g.speakerInvite = randHex(8)
	}
	if stars != nil {
		v := *stars
		g.paidStars = &v
	}
	g.version++
	up := e.fanout(g, user, g.creatorAuth, nil, nil)
	e.mu.Unlock()
	return up, nil
}

func (e *Engine) EditTitle(user, id, access int64, manage bool, title string) (*mtproto.Updates, error) {
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if !canAdmin(g, user, manage) {
		e.mu.Unlock()
		return nil, mtproto.ErrChatAdminRequired
	}
	g.title = title
	g.version++
	up := e.fanout(g, user, g.creatorAuth, nil, nil)
	e.mu.Unlock()
	return up, nil
}

func (e *Engine) ToggleRecord(user, id, access int64, manage, start, video bool, title string) (*mtproto.Updates, error) {
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if !canAdmin(g, user, manage) {
		e.mu.Unlock()
		return nil, mtproto.ErrChatAdminRequired
	}
	if start {
		g.recordStart = int32(e.now().Unix())
		g.recordVideo = video
	} else {
		g.recordStart = 0
		g.recordVideo = false
	}
	if title != "" {
		g.title = title
	}
	g.version++
	if e.media != nil {
		e.media.SetRecording(g.id, start)
	}
	up := e.fanout(g, user, g.creatorAuth, nil, nil)
	e.mu.Unlock()
	return up, nil
}

func (e *Engine) ExportInvite(user, id, access int64, manage, speaker bool) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		return "", err
	}
	if !canAdmin(g, user, manage) {
		return "", mtproto.ErrChatAdminRequired
	}
	hash := g.invite
	if speaker {
		hash = g.speakerInvite
	}
	return e.cfg.CallLinkPrefix + hash, nil
}

func (e *Engine) Invite(user, id, access int64, users []int64) (*mtproto.Updates, error) {
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	ev := &GroupEnd{
		From: user, PeerType: mtproto.PEER_USER, PeerID: 0, RandomID: newID(),
		Action: mtproto.MakeMessageActionInviteToGroupCall(inputCall(g.id, g.access), users...),
	}
	var notes []Note
	up := e.groupUpdates(user, g, nil, nil)
	for _, uid := range users {
		notes = append(notes, Note{UserID: uid, Updates: e.groupUpdates(uid, g, nil, nil)})
	}
	e.mu.Unlock()
	e.deliver(notes)
	e.finishGroup(ev, nil)
	return up, nil
}

func (e *Engine) Subscribe(user, id, access int64, on bool) (*mtproto.Updates, error) {
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if g.subs == nil {
		g.subs = map[int64]bool{}
	}
	g.subs[user] = on
	up := e.groupUpdates(user, g, nil, nil)
	e.mu.Unlock()
	return up, nil
}

func (e *Engine) StartScheduled(user, id, access int64, manage bool) (*mtproto.Updates, error) {
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if !canAdmin(g, user, manage) {
		e.mu.Unlock()
		return nil, mtproto.ErrChatAdminRequired
	}
	g.schedule = 0
	g.version++
	up := e.fanout(g, user, g.creatorAuth, nil, nil)
	ev := &GroupEnd{
		From: user, PeerType: g.peerType, PeerID: g.peerID, RandomID: newID(),
		Action: mtproto.MakeMessageActionGroupCall(inputCall(g.id, g.access), 0),
	}
	e.mu.Unlock()
	e.finishGroup(ev, nil)
	return up, nil
}

func (e *Engine) JoinPresentation(user, id, access int64, params string) (*mtproto.Updates, error) {
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	var p *part
	for _, it := range g.parts {
		if it != nil && !it.left && it.userID == user {
			p = it
			break
		}
	}
	if p == nil {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	key := partKey(p.peerType, p.peerID) + ":screen"
	e.mu.Unlock()
	answer, _, groups, err := e.answerOffer(id, key, params, strconv.FormatInt(user, 10)+":screen")
	if err != nil {
		return nil, mtproto.ErrGroupcallInvalid
	}
	e.mu.Lock()
	g, err = e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	p = g.byKey[partKey(p.peerType, p.peerID)]
	if p == nil {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	p.presentation = groups
	g.version++
	extra := []*mtproto.Update{mtproto.MakeTLUpdateGroupCallConnection(&mtproto.Update{
		Presentation: true, Params: dataJSON(answer),
	}).To_Update()}
	up := e.fanout(g, user, p.auth, []*part{p}, extra)
	e.mu.Unlock()
	return up, nil
}

func (e *Engine) LeavePresentation(user, id, access int64) (*mtproto.Updates, error) {
	e.mu.Lock()
	g, err := e.loadGroup(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	var p *part
	for _, it := range g.parts {
		if it != nil && it.userID == user && !it.left {
			p = it
			break
		}
	}
	if p == nil {
		e.mu.Unlock()
		return nil, mtproto.ErrGroupcallJoinMissing
	}
	p.presentation = nil
	g.version++
	if e.media != nil {
		e.media.Leave(g.id, partKey(p.peerType, p.peerID)+":screen")
	}
	up := e.fanout(g, user, p.auth, []*part{p}, nil)
	e.mu.Unlock()
	return up, nil
}

func (e *Engine) StreamChannels(id, access int64) (*mtproto.Phone_GroupCallStreamChannels, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, err := e.loadGroup(id, access); err != nil {
		return nil, err
	}
	ch := mtproto.MakeTLGroupCallStreamChannel(&mtproto.GroupCallStreamChannel{
		Channel: 1, Scale: 0, LastTimestampMs: e.now().UnixMilli(),
	}).To_GroupCallStreamChannel()
	return mtproto.MakeTLPhoneGroupCallStreamChannels(&mtproto.Phone_GroupCallStreamChannels{
		Channels: []*mtproto.GroupCallStreamChannel{ch},
	}).To_Phone_GroupCallStreamChannels(), nil
}

func (e *Engine) RTMP(peerType int32, peerID int64, revoke bool) (string, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	key := partKey(peerType, peerID)
	if id, ok := e.byPeer[key]; ok {
		if g := e.groups[id]; g != nil && !g.discarded {
			if revoke || g.rtmpKey == "" {
				g.rtmpKey = randHex(8)
			}
			return e.cfg.RTMPURL, g.rtmpKey
		}
	}
	cur := e.rtmpKeys[key]
	if revoke || cur == "" {
		cur = e.cfg.RTMPKey + "-" + randHex(4)
		e.rtmpKeys[key] = cur
	}
	return e.cfg.RTMPURL, cur
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
