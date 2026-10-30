package p1rest

import (
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

func (s *store) setWallpaper(self int64, in *mtproto.TLMessagesSetChatWallPaper) (*mtproto.Updates, error) {
	ref, peer, err := peerOf(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	// The method installs a wallpaper in one chat. A message id and an
	// input wallpaper are alternatives; sending both is WALLPAPER_INVALID.
	hasID := in.GetId() != nil
	hasPaper := in.GetWallpaper() != nil
	if hasID && hasPaper {
		return nil, mtproto.ErrWallpaperInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := dialogKey{user: self, kind: ref.kind, id: ref.id}
	cur := s.walls[k]
	var next *mtproto.WallPaper
	switch {
	case in.GetRevert():
		if cur.prev == nil {
			return nil, mtproto.ErrWallpaperNotFound
		}
		next = cur.prev
	case hasID:
		// The wallpaper lives on a messageActionSetChatWallPaper we do not have.
		return nil, mtproto.ErrWallpaperNotFound
	case hasPaper:
		next, err = wallpaperFrom(in.GetWallpaper(), in.GetSettings())
		if err != nil {
			return nil, err
		}
	default:
		return nil, mtproto.ErrWallpaperInvalid
	}
	prev := (*mtproto.WallPaper)(nil)
	if cur.paper != nil {
		prev = cur.paper
	}
	s.walls[k] = wallState{paper: next, prev: prev, forBoth: in.GetForBoth() && !in.GetRevert()}
	overridden := !in.GetForBoth() || in.GetRevert()
	upd := mtproto.MakeTLUpdatePeerWallpaper(&mtproto.Update{
		WallpaperOverridden: overridden,
		Peer_PEER:           peer,
		Wallpaper:           next,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(upd), nil
}

type wallState struct {
	paper   *mtproto.WallPaper
	prev    *mtproto.WallPaper
	forBoth bool
}

func wallpaperFrom(in *mtproto.InputWallPaper, settings *mtproto.WallPaperSettings) (*mtproto.WallPaper, error) {
	if in == nil {
		return nil, mtproto.ErrWallpaperInvalid
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputWallPaper:
		if in.GetId() == 0 {
			return nil, mtproto.ErrWallpaperInvalid
		}
		return mtproto.MakeTLWallPaper(&mtproto.WallPaper{
			Id:         in.GetId(),
			AccessHash: in.GetAccessHash(),
			Slug:       slugOf(in.GetId()),
			Settings:   settings,
		}).To_WallPaper(), nil
	case mtproto.Predicate_inputWallPaperSlug:
		if in.GetSlug() == "" {
			return nil, mtproto.ErrWallpaperInvalid
		}
		return mtproto.MakeTLWallPaperNoFile(&mtproto.WallPaper{
			Id:       hashSlug(in.GetSlug()),
			Slug:     in.GetSlug(),
			Settings: settings,
		}).To_WallPaper(), nil
	case mtproto.Predicate_inputWallPaperNoFile:
		if in.GetId() == 0 {
			return nil, mtproto.ErrWallpaperInvalid
		}
		return mtproto.MakeTLWallPaperNoFile(&mtproto.WallPaper{
			Id:       in.GetId(),
			Settings: settings,
		}).To_WallPaper(), nil
	default:
		return nil, mtproto.ErrWallpaperInvalid
	}
}

func slugOf(id int64) string {
	return "chat-wallpaper-" + strconv.FormatInt(id, 10)
}

func hashSlug(s string) int64 {
	var h int64 = 1469598103934665603
	for i := 0; i < len(s); i++ {
		h ^= int64(s[i])
		h *= 1099511628211
	}
	if h < 0 {
		h = -h
	}
	if h == 0 {
		h = 1
	}
	return h
}
