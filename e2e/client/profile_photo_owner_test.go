package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// Phones 9996610431 and 9996610432 are this task's range.

func TestUpdateProfilePhotoRequiresOwnership(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	owner, ownerSelf := dialPhone(t, ctx, cfg, "9996610431", "Own431")
	peer, _ := dialPhone(t, ctx, cfg, "9996610432", "Peer432")

	olderID, olderHash := uploadProfileJPEG(t, ctx, owner, 1)
	currentID, _ := uploadProfileJPEG(t, ctx, owner, 2)
	if id, _ := userPhotoFlags(t, ctx, owner, ownerSelf.ID, ownerSelf.AccessHash); id != currentID {
		t.Fatalf("avatar %d, uploaded %d", id, currentID)
	}
	foreignID, foreignHash := uploadProfileJPEG(t, ctx, peer, 3)

	rejectPhoto(t, ctx, owner, 12345, 1)
	rejectPhoto(t, ctx, owner, foreignID, foreignHash)
	rejectPhoto(t, ctx, owner, olderID, olderHash^1)
	if id, _ := userPhotoFlags(t, ctx, owner, ownerSelf.ID, ownerSelf.AccessHash); id != currentID {
		t.Fatalf("rejected update changed the avatar to %d", id)
	}
	album := albumPhotoIDs(t, ctx, owner)
	for _, id := range []int64{12345, foreignID} {
		if albumHas(album, id) {
			t.Fatalf("album absorbed %d: %v", id, album)
		}
	}
	if !albumHas(album, olderID) || !albumHas(album, currentID) {
		t.Fatalf("album lost an owned photo: %v", album)
	}

	switched, err := owner.Invoke(ctx, updateProfilePhoto(olderID, olderHash, false))
	if err != nil {
		t.Fatal(err)
	}
	if photoIDOf(t, switched) != olderID {
		t.Fatalf("switch returned %d", photoIDOf(t, switched))
	}
	if id, _ := userPhotoFlags(t, ctx, owner, ownerSelf.ID, ownerSelf.AccessHash); id != olderID {
		t.Fatalf("avatar after switch %d", id)
	}

	cleared, err := owner.Invoke(ctx, &mtproto.TLPhotosUpdateProfilePhoto{
		Constructor: mtproto.CRC32_photos_updateProfilePhoto_9e82039,
		Id:          mtproto.MakeTLInputPhotoEmpty(nil).To_InputPhoto(),
	})
	if err != nil {
		t.Fatalf("inputPhotoEmpty: %v", err)
	}
	if photoIDOf(t, cleared) == olderID {
		t.Fatal("inputPhotoEmpty left the current avatar in the result")
	}
	if id, _ := userPhotoFlags(t, ctx, owner, ownerSelf.ID, ownerSelf.AccessHash); id == olderID {
		t.Fatal("inputPhotoEmpty left the current avatar")
	}
}

func uploadProfileJPEG(t *testing.T, ctx context.Context, c *Client, seed byte) (int64, int64) {
	t.Helper()
	fileID := randomID()
	if _, err := c.Invoke(ctx, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       testJPEG(t, 320, 320, seed),
	}); err != nil {
		t.Fatal(err)
	}
	saved, err := c.Invoke(ctx, &mtproto.TLPhotosUploadProfilePhoto{
		Constructor: mtproto.CRC32_photos_uploadProfilePhoto_89f30f69,
		File: mtproto.MakeTLInputFile(&mtproto.InputFile{
			Id_INT64: fileID,
			Parts:    1,
			Name:     "avatar.jpg",
		}).To_InputFile(),
	})
	if err != nil {
		t.Fatal(err)
	}
	id := photoIDOf(t, saved)
	if id == 0 {
		t.Fatal("upload returned no photo")
	}
	return id, photoAccess(t, saved)
}

func updateProfilePhoto(id, accessHash int64, fallback bool) *mtproto.TLPhotosUpdateProfilePhoto {
	return &mtproto.TLPhotosUpdateProfilePhoto{
		Constructor: mtproto.CRC32_photos_updateProfilePhoto_9e82039,
		Fallback:    fallback,
		Id: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{
			Id:         id,
			AccessHash: accessHash,
		}).To_InputPhoto(),
	}
}

func rejectPhoto(t *testing.T, ctx context.Context, c *Client, id, accessHash int64) {
	t.Helper()
	_, err := c.Invoke(ctx, updateProfilePhoto(id, accessHash, false))
	rpc, ok := asRPCError(err)
	if !ok || rpc.Code != 400 || rpc.Message != "PHOTO_ID_INVALID" {
		t.Fatalf("photo %d: %v", id, err)
	}
}

func albumPhotoIDs(t *testing.T, ctx context.Context, c *Client) []int64 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLPhotosGetUserPhotos{
		Constructor: mtproto.CRC32_photos_getUserPhotos,
		UserId:      inputUserSelf(),
		Limit:       100,
	})
	if err != nil {
		t.Fatal(err)
	}
	var photos []*mtproto.Photo
	switch v := obj.(type) {
	case *mtproto.TLPhotosPhotos:
		photos = v.GetPhotos()
	case *mtproto.TLPhotosPhotosSlice:
		photos = v.GetPhotos()
	case *mtproto.Photos_Photos:
		photos = v.GetPhotos()
	default:
		t.Fatalf("photos.photos %T", obj)
	}
	ids := make([]int64, 0, len(photos))
	for _, photo := range photos {
		ids = append(ids, photo.GetId())
	}
	return ids
}

func albumHas(ids []int64, id int64) bool {
	for _, got := range ids {
		if got == id {
			return true
		}
	}
	return false
}
