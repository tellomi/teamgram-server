package channel

import (
	"context"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/logx"
)

// chatPhotoParallel caps concurrent media.getPhoto calls when more than one
// current avatar is resolved. channels.getFullChannel asks for one id.
const chatPhotoParallel = 8

// ChatPhotoGetter loads one stored photo, including its sizes.
// The channels BFF installs media.getPhoto. A nil getter leaves chat_photo empty.
type ChatPhotoGetter func(ctx context.Context, photoID int64) (*mtproto.Photo, error)

func (s *Service) SetChatPhotoGetter(get ChatPhotoGetter) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.photos = get
	s.mu.Unlock()
}

func (s *Service) chatPhotoGetter() ChatPhotoGetter {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.photos
}

// currentChatPhoto is channelFull.chat_photo for the channel's current avatar.
// photo id 0, a missing row, and a photo with no sizes are photoEmpty.
func (s *Service) currentChatPhoto(ctx context.Context, photoID int64) *mtproto.Photo {
	if photo := s.currentChatPhotos(ctx, []int64{photoID})[photoID]; photo != nil {
		return photo
	}
	return emptyChatFullPhoto()
}

// currentChatPhotos resolves every current avatar in one batch.
// Duplicate and zero ids do not add extra loads.
func (s *Service) currentChatPhotos(ctx context.Context, ids []int64) map[int64]*mtproto.Photo {
	uniq := uniquePhotoIDs(ids)
	out := make(map[int64]*mtproto.Photo, len(uniq))
	for _, id := range uniq {
		out[id] = emptyChatFullPhoto()
	}
	if len(uniq) == 0 {
		return out
	}
	raw := loadChatPhotos(ctx, uniq, s.chatPhotoGetter())
	for _, id := range uniq {
		if photo := fullChatPhoto(id, raw[id]); photo != nil {
			out[id] = photo
		}
	}
	return out
}

func emptyChatFullPhoto() *mtproto.Photo {
	return mtproto.MakeTLPhotoEmpty(nil).To_Photo()
}

// fullChatPhoto keeps a photo the client can open. photoEmpty and a photo
// whose id does not match the channel row are dropped.
func fullChatPhoto(photoID int64, photo *mtproto.Photo) *mtproto.Photo {
	if photo == nil || photo.GetId() != photoID || photo.GetPredicateName() != mtproto.Predicate_photo {
		return nil
	}
	if len(photo.GetSizes()) == 0 && len(photo.GetVideoSizes()) == 0 {
		return nil
	}
	return photo
}

func uniquePhotoIDs(ids []int64) []int64 {
	if len(ids) == 0 {
		return nil
	}
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

// loadChatPhotos calls get once per distinct non-zero id. The calls overlap,
// up to chatPhotoParallel, so a list does not wait one round trip per avatar.
func loadChatPhotos(ctx context.Context, ids []int64, get ChatPhotoGetter) map[int64]*mtproto.Photo {
	ids = uniquePhotoIDs(ids)
	out := make(map[int64]*mtproto.Photo, len(ids))
	if len(ids) == 0 || get == nil {
		return out
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, chatPhotoParallel)
	for _, id := range ids {
		id := id
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			photo, err := get(ctx, id)
			if err != nil {
				logx.WithContext(ctx).Errorf("channel chat_photo %d: %v", id, err)
				return
			}
			if photo == nil {
				return
			}
			mu.Lock()
			out[id] = photo
			mu.Unlock()
		}()
	}
	wg.Wait()
	return out
}
