package channel

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestFullChannelChatPhotoMatchesCard(t *testing.T) {
	svc, ctx := newTest(t)
	var calls int
	var mu sync.Mutex
	svc.SetChatPhotoGetter(func(_ context.Context, photoID int64) (*mtproto.Photo, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return storedChatPhoto(photoID, "a"), nil
	})

	created, err := svc.Create(ctx, CreateRequest{CreatorID: 1, Title: "Room", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := created.Chats[0]
	in := input(ch.Id, ch.GetAccessHash_FLAGINT64().GetValue())
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}

	before := fullFor(t, svc, ctx, 1, in)
	if before.GetFullChat().GetChatPhoto().GetPredicateName() != mtproto.Predicate_photoEmpty {
		t.Fatalf("new group chat_photo %s", before.GetFullChat().GetChatPhoto().GetPredicateName())
	}
	if calls != 0 {
		t.Fatalf("empty avatar loaded %d times", calls)
	}

	if _, err = svc.ApplyPhoto(ctx, 1, 0, in, 42, mtproto.MakeTLPhoto(&mtproto.Photo{Id: 42}).To_Photo()); err != nil {
		t.Fatal(err)
	}
	for _, viewer := range []int64{1, 2} {
		full := fullFor(t, svc, ctx, viewer, in)
		photo := full.GetFullChat().GetChatPhoto()
		card := chatIn(full.GetChats(), ch.Id)
		if photo.GetPredicateName() != mtproto.Predicate_photo || photo.GetId() != 42 {
			t.Fatalf("viewer %d chat_photo %s id %d", viewer, photo.GetPredicateName(), photo.GetId())
		}
		if len(photo.GetSizes()) == 0 {
			t.Fatalf("viewer %d chat_photo has no sizes", viewer)
		}
		if card.GetPhoto().GetPhotoId() != photo.GetId() {
			t.Fatalf("viewer %d card %d chat_photo %d", viewer, card.GetPhoto().GetPhotoId(), photo.GetId())
		}
	}
	if calls != 2 {
		t.Fatalf("loads %d, want one per viewer", calls)
	}

	batchBefore := calls
	got := svc.currentChatPhotos(ctx, []int64{42, 43, 42, 0})
	if calls-batchBefore != 2 {
		t.Fatalf("batch loads %d, want 2 distinct ids", calls-batchBefore)
	}
	if got[42].GetId() != 42 || got[43].GetId() != 43 || got[0] != nil {
		t.Fatalf("batch %+v", got)
	}

	if _, err = svc.ApplyPhoto(ctx, 1, 0, in, 0, nil); err != nil {
		t.Fatal(err)
	}
	clearedAt := calls
	for _, viewer := range []int64{1, 2} {
		full := fullFor(t, svc, ctx, viewer, in)
		photo := full.GetFullChat().GetChatPhoto()
		if photo.GetPredicateName() != mtproto.Predicate_photoEmpty {
			t.Fatalf("viewer %d after clear %s id %d", viewer, photo.GetPredicateName(), photo.GetId())
		}
		if card := chatIn(full.GetChats(), ch.Id); card.GetPhoto().GetPredicateName() != mtproto.Predicate_chatPhotoEmpty {
			t.Fatalf("viewer %d card %s", viewer, card.GetPhoto().GetPredicateName())
		}
	}
	if calls != clearedAt {
		t.Fatalf("cleared avatar loaded (%d -> %d)", clearedAt, calls)
	}
}

func TestChatPhotoLoadDropsUnusableAndOverlaps(t *testing.T) {
	svc, ctx := newTest(t)
	svc.SetChatPhotoGetter(func(_ context.Context, photoID int64) (*mtproto.Photo, error) {
		switch photoID {
		case 1:
			return nil, errors.New("media down")
		case 2:
			return mtproto.MakeTLPhotoEmpty(&mtproto.Photo{Id: 2}).To_Photo(), nil
		case 3:
			return mtproto.MakeTLPhoto(&mtproto.Photo{Id: 9, Sizes: []*mtproto.PhotoSize{photoSize("a")}}).To_Photo(), nil
		case 4:
			return mtproto.MakeTLPhoto(&mtproto.Photo{
				Id:         4,
				VideoSizes: []*mtproto.VideoSize{mtproto.MakeTLVideoSize(&mtproto.VideoSize{Type: "u", W: 32, H: 32}).To_VideoSize()},
			}).To_Photo(), nil
		default:
			return nil, nil
		}
	})
	got := svc.currentChatPhotos(ctx, []int64{1, 2, 3, 4})
	if got[1].GetPredicateName() != mtproto.Predicate_photoEmpty || got[2].GetPredicateName() != mtproto.Predicate_photoEmpty || got[3].GetPredicateName() != mtproto.Predicate_photoEmpty {
		t.Fatalf("unusable %+v", got)
	}
	if got[4].GetPredicateName() != mtproto.Predicate_photo || len(got[4].GetVideoSizes()) == 0 {
		t.Fatalf("video avatar %+v", got[4])
	}

	if len(loadChatPhotos(ctx, []int64{0, 0}, func(context.Context, int64) (*mtproto.Photo, error) {
		t.Fatal("zero id was loaded")
		return nil, nil
	})) != 0 {
		t.Fatal("zero ids produced a map entry")
	}
	if len(loadChatPhotos(ctx, []int64{7}, nil)) != 0 {
		t.Fatal("nil getter loaded a photo")
	}

	release := make(chan struct{})
	started := make(chan int64, 3)
	done := make(chan struct{})
	go func() {
		loadChatPhotos(ctx, []int64{11, 12, 11, 13}, func(_ context.Context, id int64) (*mtproto.Photo, error) {
			started <- id
			<-release
			return storedChatPhoto(id, "b"), nil
		})
		close(done)
	}()
	seen := map[int64]bool{}
	timeout := time.After(2 * time.Second)
	for len(seen) < 3 {
		select {
		case id := <-started:
			seen[id] = true
		case <-timeout:
			t.Fatalf("loads did not overlap, started %v", seen)
		}
	}
	close(release)
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("batch did not finish")
	}
}

func fullFor(t *testing.T, svc *Service, ctx context.Context, viewer int64, in *mtproto.InputChannel) *mtproto.Messages_ChatFull {
	t.Helper()
	full, err := svc.GetFullChannel(ctx, viewer, in)
	if err != nil {
		t.Fatal(err)
	}
	return full
}

func chatIn(chats []*mtproto.Chat, id int64) *mtproto.Chat {
	for _, chat := range chats {
		if chat.GetId() == id {
			return chat
		}
	}
	return nil
}

func storedChatPhoto(id int64, sizeType string) *mtproto.Photo {
	return mtproto.MakeTLPhoto(&mtproto.Photo{
		Id:         id,
		AccessHash: 7,
		Date:       9,
		DcId:       2,
		Sizes:      []*mtproto.PhotoSize{photoSize(sizeType)},
	}).To_Photo()
}

func photoSize(sizeType string) *mtproto.PhotoSize {
	return mtproto.MakeTLPhotoSize(&mtproto.PhotoSize{Type: sizeType, W: 160, H: 160, Size2: 10}).To_PhotoSize()
}
