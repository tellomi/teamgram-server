package e2e

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// Phone 9996610465 is this task's range.

func TestProfileNameRejectsEmptyAndOverlong(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, self := dialPhone(t, ctx, cfg, "9996610465", "Named")

	for _, name := range []string{"", "   "} {
		_, err := c.Invoke(ctx, &mtproto.TLAccountUpdateProfile{
			Constructor: mtproto.CRC32_account_updateProfile,
			FirstName:   strFlag(name),
		})
		if rpc, ok := asRPCError(err); !ok || rpc.Message != "FIRSTNAME_INVALID" {
			t.Fatalf("first name %q: %v", name, err)
		}
	}
	_, err := c.Invoke(ctx, &mtproto.TLAccountUpdateProfile{
		Constructor: mtproto.CRC32_account_updateProfile,
		FirstName:   strFlag(strings.Repeat("名", 65)),
	})
	if rpc, ok := asRPCError(err); !ok || rpc.Message != "FIRSTNAME_INVALID" {
		t.Fatalf("long first name: %v", err)
	}
	obj, err := c.Invoke(ctx, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUser(self.ID, self.AccessHash)},
	})
	if err != nil {
		t.Fatal(err)
	}
	user := userByID(usersOf(obj), self.ID)
	if user == nil || user.GetFirstName().GetValue() == "" || user.GetFirstName().GetValue() == strings.Repeat("名", 65) {
		t.Fatalf("stored name changed: %+v", user)
	}
}

func TestProfile320AndDelete(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	c, self := dialPhone(t, ctx, cfg, "9996610465", "Photo")
	jpeg := testJPEG(t, 320, 320, 4)
	fileID := randomID()
	if _, err := c.Invoke(ctx, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       jpeg,
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
	photoID := photoIDOf(t, saved)
	if photoID == 0 {
		t.Fatal("upload returned no photo")
	}
	if id, _ := userPhotoFlags(t, ctx, c, self.ID, self.AccessHash); id != photoID {
		t.Fatalf("user photo %d, uploaded %d", id, photoID)
	}
	access := photoAccess(t, saved)
	if _, err = c.Invoke(ctx, &mtproto.TLPhotosDeletePhotos{
		Constructor: mtproto.CRC32_photos_deletePhotos,
		Id: []*mtproto.InputPhoto{
			mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{
				Id:         photoID,
				AccessHash: access,
			}).To_InputPhoto(),
		},
	}); err != nil {
		t.Fatal(err)
	}
	if id, _ := userPhotoFlags(t, ctx, c, self.ID, self.AccessHash); id == photoID {
		t.Fatalf("deleted photo %d is still the avatar", id)
	}
}

func TestUnsaveMusic(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, _ := dialPhone(t, ctx, cfg, "9996610465", "Music")
	const docID = int64(8800465)
	doc := mtproto.MakeTLInputDocument(&mtproto.InputDocument{
		Id:         docID,
		AccessHash: 1,
	}).To_InputDocument()
	if _, err := c.Invoke(ctx, &mtproto.TLAccountSaveMusic{
		Constructor: mtproto.CRC32_account_saveMusic,
		Id:          doc,
		Unsave:      false,
	}); err != nil {
		t.Fatal(err)
	}
	if !savedMusicHas(t, ctx, c, docID) {
		t.Fatal("save did not store the document id")
	}
	if _, err := c.Invoke(ctx, &mtproto.TLAccountSaveMusic{
		Constructor: mtproto.CRC32_account_saveMusic,
		Id:          doc,
		Unsave:      true,
	}); err != nil {
		t.Fatal(err)
	}
	if savedMusicHas(t, ctx, c, docID) {
		t.Fatal("unsave left the document id")
	}
}

func photoAccess(t *testing.T, obj mtproto.TLObject) int64 {
	t.Helper()
	var photo *mtproto.Photo
	switch v := obj.(type) {
	case *mtproto.TLPhotosPhoto:
		photo = v.GetPhoto()
	case *mtproto.Photos_Photo:
		photo = v.GetPhoto()
	default:
		t.Fatalf("photos.photo %T", obj)
	}
	return photo.GetAccessHash()
}

func savedMusicHas(t *testing.T, ctx context.Context, c *Client, id int64) bool {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountGetSavedMusicIds{
		Constructor: mtproto.CRC32_account_getSavedMusicIds,
		Hash:        0,
	})
	if err != nil {
		t.Fatal(err)
	}
	var ids []int64
	switch v := obj.(type) {
	case *mtproto.TLAccountSavedMusicIds:
		ids = v.GetIds()
	case *mtproto.Account_SavedMusicIds:
		ids = v.GetIds()
	default:
		t.Fatalf("saved music ids %T", obj)
	}
	for _, got := range ids {
		if got == id {
			return true
		}
	}
	return false
}
