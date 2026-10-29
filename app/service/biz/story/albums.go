package story

import (
	"context"
	"hash/fnv"
	"strconv"
	"strings"

	"github.com/teamgram/proto/mtproto"
)

func (s *Service) CreateAlbum(ctx context.Context, actor int64, owner Owner, accessHash int64, title string, stories []int32) (*mtproto.StoryAlbum, error) {
	if err := s.canEdit(ctx, actor, owner, accessHash); err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	if title == "" || len(title) > 128 {
		return nil, mtproto.ErrTitleInvalid
	}
	ids, err := s.knownStories(ctx, owner, stories)
	if err != nil {
		return nil, err
	}
	album := &Album{Owner: owner, Title: title, StoryIDs: ids}
	if err = s.store.CreateAlbum(ctx, album); err != nil {
		return nil, err
	}
	return s.albumTL(ctx, album), nil
}

func (s *Service) UpdateAlbum(ctx context.Context, actor int64, owner Owner, accessHash int64, albumID int32, title *string, deleteIDs, addIDs, order []int32) (*mtproto.StoryAlbum, error) {
	if err := s.canEdit(ctx, actor, owner, accessHash); err != nil {
		return nil, err
	}
	album, err := s.store.GetAlbum(ctx, owner, albumID)
	if err != nil {
		return nil, err
	}
	if album == nil {
		return nil, errAlbumInvalid
	}
	if title != nil {
		t := strings.TrimSpace(*title)
		if t == "" || len(t) > 128 {
			return nil, mtproto.ErrTitleInvalid
		}
		album.Title = t
	}
	if len(deleteIDs) > 0 {
		drop := map[int32]bool{}
		for _, id := range deleteIDs {
			drop[id] = true
		}
		var keep []int32
		for _, id := range album.StoryIDs {
			if !drop[id] {
				keep = append(keep, id)
			}
		}
		album.StoryIDs = keep
	}
	if len(addIDs) > 0 {
		ids, err := s.knownStories(ctx, owner, addIDs)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if !contains32(album.StoryIDs, id) {
				album.StoryIDs = append(album.StoryIDs, id)
			}
		}
	}
	if len(order) > 0 {
		sorted := append([]int32(nil), album.StoryIDs...)
		pos := map[int32]int{}
		for i, id := range order {
			pos[id] = i
		}
		var head, tail []int32
		for _, id := range sorted {
			if _, ok := pos[id]; ok {
				head = append(head, id)
			} else {
				tail = append(tail, id)
			}
		}
		for i := 1; i < len(head); i++ {
			j := i
			for j > 0 && pos[head[j]] < pos[head[j-1]] {
				head[j], head[j-1] = head[j-1], head[j]
				j--
			}
		}
		album.StoryIDs = append(head, tail...)
	}
	if err = s.store.SaveAlbum(ctx, album); err != nil {
		return nil, err
	}
	return s.albumTL(ctx, album), nil
}

func (s *Service) ReorderAlbums(ctx context.Context, actor int64, owner Owner, accessHash int64, order []int32) error {
	if err := s.canEdit(ctx, actor, owner, accessHash); err != nil {
		return err
	}
	albums, err := s.store.ListAlbums(ctx, owner)
	if err != nil {
		return err
	}
	rank := map[int32]int{}
	for i, id := range order {
		rank[id] = i
	}
	for _, album := range albums {
		if n, ok := rank[album.ID]; ok {
			album.Order = int32(n + 1)
		} else {
			album.Order = int32(len(order) + int(album.Order) + 1)
		}
		if err = s.store.SaveAlbum(ctx, album); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) DeleteAlbum(ctx context.Context, actor int64, owner Owner, accessHash int64, albumID int32) error {
	if err := s.canEdit(ctx, actor, owner, accessHash); err != nil {
		return err
	}
	album, err := s.store.GetAlbum(ctx, owner, albumID)
	if err != nil {
		return err
	}
	if album == nil {
		return errAlbumInvalid
	}
	return s.store.DeleteAlbum(ctx, owner, albumID)
}

func (s *Service) GetAlbums(ctx context.Context, viewer int64, owner Owner, accessHash int64, hash int64) (*mtproto.Stories_Albums, error) {
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	albums, err := s.store.ListAlbums(ctx, owner)
	if err != nil {
		return nil, err
	}
	sum := albumsHash(albums)
	if hash != 0 && hash == sum {
		return mtproto.MakeTLStoriesAlbumsNotModified(&mtproto.Stories_Albums{}).To_Stories_Albums(), nil
	}
	out := make([]*mtproto.StoryAlbum, 0, len(albums))
	for _, album := range albums {
		out = append(out, s.albumTL(ctx, album))
	}
	return mtproto.MakeTLStoriesAlbums(&mtproto.Stories_Albums{
		Hash:   sum,
		Albums: out,
	}).To_Stories_Albums(), nil
}

func (s *Service) GetAlbumStories(ctx context.Context, viewer int64, owner Owner, accessHash int64, albumID, offset, limit int32) (*mtproto.Stories_Stories, error) {
	if err := s.canViewPeer(ctx, viewer, owner, accessHash); err != nil {
		return nil, err
	}
	album, err := s.store.GetAlbum(ctx, owner, albumID)
	if err != nil {
		return nil, err
	}
	if album == nil {
		return nil, errAlbumInvalid
	}
	var recs []*Record
	for _, id := range album.StoryIDs {
		rec, err := s.store.Get(ctx, owner, id)
		if err != nil {
			return nil, err
		}
		if rec == nil || rec.Deleted || !s.privacyAllows(ctx, rec, viewer) {
			continue
		}
		recs = append(recs, rec)
	}
	total := int32(len(recs))
	if offset > 0 {
		cut := 0
		for cut < len(recs) && recs[cut].id() != offset {
			cut++
		}
		if cut < len(recs) {
			recs = recs[cut+1:]
		}
	}
	limit = clampLimit(limit, 50, 100)
	if int(limit) < len(recs) {
		recs = recs[:limit]
	}
	box := s.storiesBox(ctx, viewer, owner, recs)
	box.Count = total
	return box, nil
}

func (s *Service) knownStories(ctx context.Context, owner Owner, ids []int32) ([]int32, error) {
	var out []int32
	for _, id := range ids {
		rec, err := s.store.Get(ctx, owner, id)
		if err != nil {
			return nil, err
		}
		if rec == nil || rec.Deleted {
			return nil, errStoryIDInvalid
		}
		out = append(out, id)
	}
	return out, nil
}

func (s *Service) albumTL(ctx context.Context, album *Album) *mtproto.StoryAlbum {
	tl := mtproto.MakeTLStoryAlbum(&mtproto.StoryAlbum{
		AlbumId: album.ID,
		Title:   album.Title,
	}).To_StoryAlbum()
	for _, id := range album.StoryIDs {
		rec, err := s.store.Get(ctx, album.Owner, id)
		if err != nil || rec == nil || rec.Deleted || rec.Item.Media == nil {
			continue
		}
		if rec.Item.Media.GetPhoto_FLAGPHOTO() != nil {
			tl.IconPhoto = rec.Item.Media.GetPhoto_FLAGPHOTO()
			break
		}
		if rec.Item.Media.GetDocument() != nil {
			tl.IconVideo = rec.Item.Media.GetDocument()
			break
		}
	}
	return tl
}

func albumsHash(albums []*Album) int64 {
	h := fnv.New64a()
	for _, a := range albums {
		_, _ = h.Write([]byte(strconv.FormatInt(int64(a.ID), 10)))
		_, _ = h.Write([]byte(a.Title))
		for _, id := range a.StoryIDs {
			_, _ = h.Write([]byte(strconv.FormatInt(int64(id), 10)))
		}
		_, _ = h.Write([]byte{0})
	}
	return int64(h.Sum64())
}
