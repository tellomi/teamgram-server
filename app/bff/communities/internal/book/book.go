// Package book stores communities for one BFF process.
//
// communities.create and the toggle methods write here. A restart drops the
// book. Chat objects for foreign linked peers are not loaded, so result
// vectors that would carry those chats stay empty and the ids are returned.
package book

import (
	"crypto/rand"
	"encoding/binary"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	maxTitle = 128
	maxAbout = 255
	idBase   = int64(9_000_000_000_001)
)

type peerLink struct {
	peerType    int32
	peerID      int64
	visible     bool
	pending     bool
	requestedBy int64
	date        int32
}

type community struct {
	id        int64
	hash      int64
	owner     int64
	title     string
	about     string
	date      int32
	collapsed bool
	links     []*peerLink
	banned    map[int64]bool
	members   map[int64]bool
}

// Book is the process-local community store.
type Book struct {
	mu   sync.Mutex
	byID map[int64]*community
	next int64
	now  func() time.Time
}

// New returns an empty book.
func New() *Book {
	return &Book{
		byID: map[int64]*community{},
		next: idBase,
		now:  time.Now,
	}
}

var shared = New()

// Shared is the book the BFF handlers use.
func Shared() *Book {
	return shared
}

// ResetShared drops process state. Tests call it so cases do not leak.
func ResetShared() {
	shared = New()
}

// Create builds a community and links peer. hidden stores the first link as
// not visible. The result is updates with the community chat and updateChannel.
// Neither carries pts, and the box seq is 0, so the client's difference cursor
// does not move.
func (b *Book) Create(owner int64, title, about string, hidden bool, peer *mtproto.PeerUtil) (*mtproto.Updates, error) {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil, mtproto.ErrChatTitleEmpty
	}
	if utf8.RuneCountInString(title) > maxTitle {
		return nil, mtproto.ErrTitleInvalid
	}
	if mtproto.UTF16Len(about) > maxAbout {
		return nil, mtproto.ErrAboutTooLong
	}
	p, err := requirePeer(peer)
	if err != nil {
		return nil, err
	}
	hash, err := newHash()
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c := &community{
		id:      b.next,
		hash:    hash,
		owner:   owner,
		title:   title,
		about:   about,
		date:    int32(b.now().Unix()),
		banned:  map[int64]bool{},
		members: map[int64]bool{owner: true},
	}
	b.next++
	c.links = []*peerLink{{
		peerType:    p.PeerType,
		peerID:      p.PeerId,
		visible:     !hidden,
		requestedBy: owner,
		date:        c.date,
	}}
	b.byID[c.id] = c
	return c.updates(owner), nil
}

// Joined is messages.chats of communities this user created.
func (b *Book) Joined(user int64) *mtproto.Messages_Chats {
	b.mu.Lock()
	defer b.mu.Unlock()
	chats := []*mtproto.Chat{}
	for _, c := range b.list() {
		if c.members[user] && !c.banned[user] {
			chats = append(chats, c.chat(user))
		}
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats()
}

// TogglePeerLink changes one linked peer. The owner applies visible, hidden,
// or deleted immediately. Another user files a request the owner approves later.
func (b *Book) TogglePeerLink(user int64, communityID, hash int64, peer *mtproto.PeerUtil, visible, hidden, deleted bool) error {
	p, err := requirePeer(peer)
	if err != nil {
		return err
	}
	if !visible && !hidden && !deleted {
		return bad("PEER_LINK_INVALID")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c, err := b.lookup(communityID, hash)
	if err != nil {
		return err
	}
	if user != c.owner {
		if deleted || c.banned[user] {
			return mtproto.ErrChatAdminRequired
		}
		c.upsert(p, user, true, visible && !hidden)
		return nil
	}
	if deleted {
		c.remove(p)
		return nil
	}
	c.upsert(p, user, false, visible && !hidden)
	return nil
}

// SetCollapsed stores the dialogs flag and returns an updates box with the
// community chat. updateChannel has no pts.
func (b *Book) SetCollapsed(user int64, communityID, hash int64, collapsed bool) (*mtproto.Updates, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, err := b.lookup(communityID, hash)
	if err != nil {
		return nil, err
	}
	if !c.members[user] || c.banned[user] {
		return nil, mtproto.ErrChannelPrivate
	}
	c.collapsed = collapsed
	return c.updates(user), nil
}

// Requests pages pending peer-link requests. The owner is the only reader.
func (b *Book) Requests(user int64, communityID, hash int64, offset string, limit int32) (*mtproto.Communities_PeerLinkRequests, error) {
	start, err := pageOffset(offset)
	if err != nil {
		return nil, err
	}
	n, err := pageLimit(limit)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c, err := b.lookup(communityID, hash)
	if err != nil {
		return nil, err
	}
	if user != c.owner {
		return nil, mtproto.ErrChatAdminRequired
	}
	pending := c.pending()
	total := int32(len(pending))
	if start > len(pending) {
		start = len(pending)
	}
	end := start + n
	if end > len(pending) {
		end = len(pending)
	}
	page := pending[start:end]
	reqs := make([]*mtproto.CommunityPeerRequest, 0, len(page))
	for _, link := range page {
		reqs = append(reqs, mtproto.MakeTLCommunityPeerRequest(&mtproto.CommunityPeerRequest{
			Visible:     link.visible,
			Peer:        mtproto.MakePeer(link.peerType, link.peerID),
			RequestedBy: link.requestedBy,
			Date:        link.date,
		}).To_CommunityPeerRequest())
	}
	var next *wrapperspb.StringValue
	if end < len(pending) {
		next = wrapperspb.String(strconv.Itoa(end))
	}
	return mtproto.MakeTLCommunitiesPeerLinkRequests(&mtproto.Communities_PeerLinkRequests{
		TotalCount: total,
		Requests:   reqs,
		NextOffset: next,
		Chats:      []*mtproto.Chat{},
		Users:      []*mtproto.User{},
	}).To_Communities_PeerLinkRequests(), nil
}

// DecideRequest approves or rejects one pending link.
func (b *Book) DecideRequest(user int64, communityID, hash int64, peer *mtproto.PeerUtil, reject bool) error {
	p, err := requirePeer(peer)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c, err := b.lookup(communityID, hash)
	if err != nil {
		return err
	}
	if user != c.owner {
		return mtproto.ErrChatAdminRequired
	}
	link := c.find(p)
	if link == nil || !link.pending {
		return mtproto.ErrPeerIdInvalid
	}
	if reject {
		c.remove(p)
		return nil
	}
	link.pending = false
	return nil
}

// DecideAll approves or rejects every pending link.
func (b *Book) DecideAll(user int64, communityID, hash int64, reject bool) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	c, err := b.lookup(communityID, hash)
	if err != nil {
		return err
	}
	if user != c.owner {
		return mtproto.ErrChatAdminRequired
	}
	if reject {
		next := c.links[:0]
		for _, link := range c.links {
			if !link.pending {
				next = append(next, link)
			}
		}
		c.links = next
		return nil
	}
	for _, link := range c.links {
		link.pending = false
	}
	return nil
}

// SetBanned bans or unbans a user. The owner cannot be banned.
func (b *Book) SetBanned(user int64, communityID, hash int64, participant *mtproto.PeerUtil, unban bool) error {
	if participant == nil || (participant.PeerType != mtproto.PEER_USER && participant.PeerType != mtproto.PEER_SELF) || participant.PeerId == 0 {
		return mtproto.ErrUserIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c, err := b.lookup(communityID, hash)
	if err != nil {
		return err
	}
	if user != c.owner {
		return mtproto.ErrChatAdminRequired
	}
	if participant.PeerId == c.owner {
		return mtproto.ErrUserCreator
	}
	if unban {
		delete(c.banned, participant.PeerId)
		return nil
	}
	c.banned[participant.PeerId] = true
	delete(c.members, participant.PeerId)
	return nil
}

// ParticipantChats lists linked chat and channel ids for one participant.
// chats and users stay empty: this process does not load those peers.
func (b *Book) ParticipantChats(viewer int64, communityID, hash int64, participant *mtproto.PeerUtil) (*mtproto.Communities_ParticipantJoinedChats, error) {
	if participant == nil || (participant.PeerType != mtproto.PEER_USER && participant.PeerType != mtproto.PEER_SELF) || participant.PeerId == 0 {
		return nil, mtproto.ErrUserIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	c, err := b.lookup(communityID, hash)
	if err != nil {
		return nil, err
	}
	if viewer != c.owner && viewer != participant.PeerId {
		return nil, mtproto.ErrChannelPrivate
	}
	if c.banned[participant.PeerId] && viewer == participant.PeerId {
		return nil, mtproto.ErrUserBannedInChannel
	}
	var creator, joined []int64
	for _, link := range c.links {
		if link.pending || (link.peerType != mtproto.PEER_CHAT && link.peerType != mtproto.PEER_CHANNEL) {
			continue
		}
		if link.requestedBy == participant.PeerId {
			creator = append(creator, link.peerID)
			continue
		}
		if c.members[participant.PeerId] && !c.banned[participant.PeerId] {
			joined = append(joined, link.peerID)
		}
	}
	if creator == nil {
		creator = []int64{}
	}
	if joined == nil {
		joined = []int64{}
	}
	return mtproto.MakeTLCommunitiesParticipantJoinedChats(&mtproto.Communities_ParticipantJoinedChats{
		CreatorChatIds: creator,
		JoinedChatIds:  joined,
		Chats:          []*mtproto.Chat{},
		Users:          []*mtproto.User{},
	}).To_Communities_ParticipantJoinedChats(), nil
}

func (b *Book) lookup(id, hash int64) (*community, error) {
	c := b.byID[id]
	if c == nil || c.hash != hash {
		return nil, mtproto.ErrChannelInvalid
	}
	return c, nil
}

func (b *Book) list() []*community {
	out := make([]*community, 0, len(b.byID))
	for _, c := range b.byID {
		out = append(out, c)
	}
	for i := 1; i < len(out); i++ {
		j := i
		for j > 0 && out[j].id < out[j-1].id {
			out[j], out[j-1] = out[j-1], out[j]
			j--
		}
	}
	return out
}

func (c *community) find(p *mtproto.PeerUtil) *peerLink {
	for _, link := range c.links {
		if link.peerType == p.PeerType && link.peerID == p.PeerId {
			return link
		}
	}
	return nil
}

func (c *community) upsert(p *mtproto.PeerUtil, by int64, pending, visible bool) {
	if link := c.find(p); link != nil {
		link.pending = pending
		link.visible = visible
		link.requestedBy = by
		link.date = int32(time.Now().Unix())
		return
	}
	c.links = append(c.links, &peerLink{
		peerType:    p.PeerType,
		peerID:      p.PeerId,
		visible:     visible,
		pending:     pending,
		requestedBy: by,
		date:        int32(time.Now().Unix()),
	})
}

func (c *community) remove(p *mtproto.PeerUtil) {
	next := c.links[:0]
	for _, link := range c.links {
		if link.peerType == p.PeerType && link.peerID == p.PeerId {
			continue
		}
		next = append(next, link)
	}
	c.links = next
}

func (c *community) pending() []*peerLink {
	out := []*peerLink{}
	for _, link := range c.links {
		if link.pending {
			out = append(out, link)
		}
	}
	return out
}

func (c *community) chat(viewer int64) *mtproto.Chat {
	creator := viewer == c.owner
	ch := &mtproto.Chat{
		Creator:              creator,
		CollapsedInDialogs:   c.collapsed,
		Id:                   c.id,
		AccessHash_FLAGINT64: wrapperspb.Int64(c.hash),
		Title:                c.title,
		Photo:                mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto(),
		Date:                 c.date,
	}
	if creator {
		ch.AdminRights = mtproto.MakeDefaultChatAdminRights()
	}
	return mtproto.MakeTLCommunity(ch).To_Chat()
}

func (c *community) updates(viewer int64) *mtproto.Updates {
	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{c.chat(viewer)},
		mtproto.MakeTLUpdateChannel(&mtproto.Update{ChannelId: c.id}).To_Update(),
	)
}

func requirePeer(p *mtproto.PeerUtil) (*mtproto.PeerUtil, error) {
	if p == nil || p.PeerId == 0 {
		return nil, mtproto.ErrPeerIdInvalid
	}
	switch p.PeerType {
	case mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
		return p, nil
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
}

func newHash() (int64, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, mtproto.ErrInternalServerError
	}
	v := int64(binary.BigEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
	if v == 0 {
		return 1, nil
	}
	return v, nil
}

func pageOffset(offset string) (int, error) {
	if offset == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(offset)
	if err != nil || n < 0 {
		return 0, mtproto.ErrOffsetInvalid
	}
	return n, nil
}

func pageLimit(limit int32) (int, error) {
	if limit == 0 {
		return 20, nil
	}
	if limit < 0 || limit > 100 {
		return 0, mtproto.ErrLimitInvalid
	}
	return int(limit), nil
}

func bad(code string) error {
	return status.Error(mtproto.ErrBadRequest, code)
}
