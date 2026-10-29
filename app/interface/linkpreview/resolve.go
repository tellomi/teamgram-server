package linkpreview

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

const (
	ownerUser    int8 = 1
	ownerChannel int8 = 2
	feedScan          = 20
	feedKeep          = 10
)

// Service resolves public tell.cc paths.
type Service struct {
	cat Catalog
	cfg Config
	now func() time.Time
}

func New(cfg Config, cat Catalog) *Service {
	if cat == nil {
		cat = NewMemory()
	}
	cfg = cfg.withDefaults()
	return &Service{cat: cat, cfg: cfg, now: time.Now}
}

func (s *Service) SetClock(now func() time.Time) {
	if now != nil {
		s.now = now
	}
}

func (s *Service) Resolve(ctx context.Context, raw string, extra url.Values) (*Card, error) {
	rt, err := parseRequest(raw, extra)
	if err != nil {
		return nil, err
	}
	switch rt.op {
	case opProxy:
		return s.proxy(rt)
	case opSocks:
		return s.socks(rt)
	case opShare:
		return s.share(rt)
	case opBoost:
		return s.boost(ctx, rt)
	case opInvite:
		return s.invite(ctx, rt)
	case opStickers:
		return s.pack(ctx, rt, false)
	case opEmoji:
		return s.pack(ctx, rt, true)
	case opUser:
		return s.user(ctx, rt)
	case opFeed:
		return s.feed(ctx, rt)
	case opMessage:
		return s.messageByUsername(ctx, rt)
	case opCMessage:
		return s.messageByChannel(ctx, rt)
	case opStory:
		return s.story(ctx, rt)
	default:
		return nil, errInvalid
	}
}

func (s *Service) link(path string) string {
	return s.cfg.LinkBaseURL + path
}

func (s *Service) finish(card *Card) *Card {
	card.OK = true
	card.SchemaVersion = SchemaVersion
	if card.Path != "" {
		card.Link = s.link(card.Path)
	}
	return card
}

func (s *Service) fileURL(ctx context.Context, path string, photoID, documentID int64) string {
	if path == "" && (photoID != 0 || documentID != 0) {
		got, err := s.cat.FilePath(ctx, photoID, documentID)
		if err == nil {
			path = got
		}
	}
	return mediaURL(s.cfg, path, s.now())
}

func (s *Service) peerJSON(ctx context.Context, p *storedPeer) *Peer {
	out := &Peer{
		Kind:        p.Kind,
		Title:       p.Title,
		Username:    p.Username,
		About:       p.About,
		Verified:    p.Verified,
		Scam:        p.Scam,
		Fake:        p.Fake,
		Premium:     (p.Kind == KindUser || p.Kind == KindBot) && premium.IsMember(p.Premium, 0, time.Now().Unix()),
		MemberCount: p.MemberCount,
		Broadcast:   p.Broadcast,
		Megagroup:   p.Megagroup,
		AvatarURL:   s.fileURL(ctx, p.AvatarPath, p.PhotoID, 0),
	}
	return out
}

func (s *Service) fillPost(ctx context.Context, post *Post) {
	post.MediaURL = s.fileURL(ctx, "", post.mediaPhotoID, post.mediaDocumentID)
}

func (s *Service) user(ctx context.Context, rt *route) (*Card, error) {
	p, err := s.cat.PeerByUsername(ctx, rt.username)
	if err != nil {
		return nil, err
	}
	peer := s.peerJSON(ctx, p)
	path := "/" + p.Username
	if p.Kind == KindBot {
		st, err := startFrom(rt.query)
		if err != nil {
			return nil, err
		}
		if st != nil {
			peer.Start = st
			path += "?" + st.Param + "=" + url.QueryEscape(st.Value)
		}
	}
	return s.finish(&Card{Kind: p.Kind, Path: path, Peer: peer}), nil
}

func (s *Service) invite(ctx context.Context, rt *route) (*Card, error) {
	inv, err := s.cat.Invite(ctx, rt.hash)
	if err != nil {
		return nil, err
	}
	path := "/+" + rt.hash
	card := &Card{
		Kind: KindInvite,
		Path: path,
		Invite: &Invite{
			Title:         inv.Title,
			About:         inv.About,
			AvatarURL:     s.fileURL(ctx, inv.AvatarPath, inv.PhotoID, 0),
			MemberCount:   inv.MemberCount,
			RequestNeeded: inv.RequestNeeded,
			Broadcast:     inv.Broadcast,
			Megagroup:     inv.Megagroup,
		},
	}
	return s.finish(card), nil
}

func (s *Service) messageByUsername(ctx context.Context, rt *route) (*Card, error) {
	p, err := s.cat.PeerByUsername(ctx, rt.username)
	if err != nil {
		return nil, err
	}
	if p.Kind != KindChannel && p.Kind != KindGroup {
		return nil, errNotFound
	}
	return s.message(ctx, p, rt)
}

func (s *Service) messageByChannel(ctx context.Context, rt *route) (*Card, error) {
	p, err := s.cat.PeerByChannelID(ctx, rt.id)
	if err != nil {
		return nil, err
	}
	return s.message(ctx, p, rt)
}

func (s *Service) message(ctx context.Context, p *storedPeer, rt *route) (*Card, error) {
	raw, err := s.cat.Message(ctx, p.ID, rt.msgID)
	if err != nil {
		return nil, err
	}
	post, ok := renderPost(raw.Msg)
	if !ok {
		return nil, errNotFound
	}
	s.fillPost(ctx, &post)
	path := messagePath(p, rt)
	return s.finish(&Card{Kind: KindMessage, Path: path, Peer: s.peerJSON(ctx, p), Post: &post}), nil
}

func messagePath(p *storedPeer, rt *route) string {
	id := strconv.FormatInt(int64(rt.msgID), 10)
	switch rt.form {
	case formWeb:
		return "/s/" + p.Username + "/" + id
	case formC:
		return "/c/" + strconv.FormatInt(p.ID, 10) + "/" + id
	default:
		return "/" + p.Username + "/" + id
	}
}

func (s *Service) feed(ctx context.Context, rt *route) (*Card, error) {
	p, err := s.cat.PeerByUsername(ctx, rt.username)
	if err != nil {
		return nil, err
	}
	if p.Kind != KindChannel && p.Kind != KindGroup {
		return nil, errNotFound
	}
	raw, err := s.cat.Latest(ctx, p.ID, feedScan)
	if err != nil {
		return nil, err
	}
	posts := make([]Post, 0, feedKeep)
	for _, item := range raw {
		post, ok := renderPost(item.Msg)
		if !ok {
			continue
		}
		s.fillPost(ctx, &post)
		posts = append(posts, post)
		if len(posts) == feedKeep {
			break
		}
	}
	return s.finish(&Card{
		Kind:  KindFeed,
		Path:  "/s/" + p.Username,
		Peer:  s.peerJSON(ctx, p),
		Posts: &posts,
	}), nil
}

func (s *Service) story(ctx context.Context, rt *route) (*Card, error) {
	p, err := s.cat.PeerByUsername(ctx, rt.username)
	if err != nil {
		return nil, err
	}
	kind := ownerUser
	if p.Kind == KindChannel || p.Kind == KindGroup {
		kind = ownerChannel
	}
	rec, err := s.cat.Story(ctx, kind, p.ID, rt.storyID)
	if err != nil {
		return nil, err
	}
	now := int32(s.now().Unix())
	if !storyVisible(rec.Item, rec.Deleted, now) {
		return nil, errNotFound
	}
	view := storyView(rec.Item)
	view.MediaURL = s.fileURL(ctx, "", view.mediaPhotoID, view.mediaDocumentID)
	return s.finish(&Card{
		Kind:  KindStory,
		Path:  "/" + p.Username + "/s/" + strconv.FormatInt(int64(rt.storyID), 10),
		Peer:  s.peerJSON(ctx, p),
		Story: &view,
	}), nil
}

func storyView(item *mtproto.StoryItem) Story {
	st := Story{
		StoryID:    item.Id,
		Date:       item.Date,
		ExpireDate: item.ExpireDate,
		Pinned:     item.Pinned,
		Caption:    storyCaption(item),
	}
	if item.Media == nil {
		return st
	}
	post, ok := renderPost(&mtproto.Message{Id: item.Id, Date: item.Date, Media: item.Media})
	if !ok {
		return st
	}
	st.MediaType = post.MediaType
	st.mediaPhotoID = post.mediaPhotoID
	st.mediaDocumentID = post.mediaDocumentID
	return st
}

func (s *Service) pack(ctx context.Context, rt *route, emoji bool) (*Card, error) {
	p, err := s.cat.Pack(ctx, rt.short, emoji)
	if err != nil {
		return nil, err
	}
	kind := KindStickerSet
	prefix := "/addstickers/"
	if emoji {
		kind = KindEmojiSet
		prefix = "/addemoji/"
	}
	return s.finish(&Card{
		Kind: kind,
		Path: prefix + p.ShortName,
		Pack: &Pack{
			ShortName: p.ShortName,
			Title:     p.Title,
			Count:     p.Count,
			Emoji:     emoji,
			AvatarURL: s.fileURL(ctx, p.AvatarPath, 0, 0),
		},
	}), nil
}

func (s *Service) boost(ctx context.Context, rt *route) (*Card, error) {
	if rt.username == "" {
		return s.finish(&Card{Kind: KindBoost, Path: "/boost"}), nil
	}
	p, err := s.cat.PeerByUsername(ctx, rt.username)
	if err != nil {
		return nil, err
	}
	if p.Kind != KindChannel && p.Kind != KindGroup {
		return nil, errNotFound
	}
	return s.finish(&Card{
		Kind: KindBoost,
		Path: "/boost/" + p.Username,
		Peer: s.peerJSON(ctx, p),
	}), nil
}

func (s *Service) proxy(rt *route) (*Card, error) {
	server := strings.TrimSpace(rt.query.Get("server"))
	port, err := parsePort(rt.query.Get("port"))
	secret := rt.query.Get("secret")
	if err != nil || !validHost(server) || !validSecret(secret) {
		return nil, errInvalid
	}
	q := url.Values{}
	q.Set("server", server)
	q.Set("port", strconv.Itoa(port))
	q.Set("secret", secret)
	return s.finish(&Card{
		Kind:  KindProxy,
		Path:  "/proxy?" + q.Encode(),
		Proxy: &Proxy{Server: server, Port: port, Secret: secret},
	}), nil
}

func (s *Service) socks(rt *route) (*Card, error) {
	server := strings.TrimSpace(rt.query.Get("server"))
	port, err := parsePort(rt.query.Get("port"))
	if err != nil || !validHost(server) {
		return nil, errInvalid
	}
	user := rt.query.Get("user")
	pass := rt.query.Get("pass")
	if len(user) > 255 || len(pass) > 255 {
		return nil, errInvalid
	}
	q := url.Values{}
	q.Set("server", server)
	q.Set("port", strconv.Itoa(port))
	if user != "" {
		q.Set("user", user)
	}
	if pass != "" {
		q.Set("pass", pass)
	}
	return s.finish(&Card{
		Kind:  KindSocks,
		Path:  "/socks?" + q.Encode(),
		Socks: &Socks{Server: server, Port: port, User: user, Pass: pass},
	}), nil
}

func (s *Service) share(rt *route) (*Card, error) {
	raw := strings.TrimSpace(rt.query.Get("url"))
	if raw == "" || len(raw) > 2048 {
		return nil, errInvalid
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, errInvalid
	}
	text := capRunes(rt.query.Get("text"), 500)
	q := url.Values{}
	q.Set("url", u.String())
	if text != "" {
		q.Set("text", text)
	}
	return s.finish(&Card{
		Kind:  KindShare,
		Path:  "/share/url?" + q.Encode(),
		Share: &Share{URL: u.String(), Text: text},
	}), nil
}

func parsePort(s string) (int, error) {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n < 1 || n > 65535 {
		return 0, errInvalid
	}
	return n, nil
}
