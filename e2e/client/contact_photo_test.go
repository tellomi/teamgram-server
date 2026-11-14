package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestContactProfilePhotoSaveSuggestAndClear follows Telegram Desktop:
// save with a file shows up as userFull.personal_photo for the caller only,
// save with no file clears it, and suggest delivers
// messageActionSuggestProfilePhoto to the contact.
func TestContactProfilePhotoSaveSuggestAndClear(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	contact := inputUser(bSelf.ID, bSelf.AccessHash)
	peerForB := inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)

	// Profile photos are cropped to the a/b/c ladder. A 32px image is scaled
	// up to 640 and the resizer rejects it with IMAGE_PROCESS_FAILED.
	jpeg := testJPEG(t, 800, 800, 9)
	fileID := randomID()
	if _, err := a.Invoke(ctx, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       jpeg,
	}); err != nil {
		t.Fatal(err)
	}
	file := mtproto.MakeTLInputFile(&mtproto.InputFile{
		Id_INT64: fileID,
		Parts:    1,
		Name:     "contact.jpg",
	}).To_InputFile()

	saved, err := a.Invoke(ctx, &mtproto.TLPhotosUploadContactProfilePhoto{
		Constructor: mtproto.CRC32_photos_uploadContactProfilePhoto_e14c4a71,
		UserId:      contact,
		Save:        true,
		File:        file,
	})
	if err != nil {
		t.Fatal(err)
	}
	photoID := photoIDOf(t, saved)
	if photoID == 0 {
		t.Fatalf("save returned %T without a photo", saved)
	}
	if got := personalPhotoID(t, ctx, a, bSelf.ID, bSelf.AccessHash); got != photoID {
		t.Fatalf("caller personal_photo %d, saved %d", got, photoID)
	}
	if got := personalPhotoID(t, ctx, b, aliceSeen.ID, aliceSeen.AccessHash); got != 0 {
		t.Fatalf("contact saw the caller's personal photo %d", got)
	}

	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		_, _ = a.Invoke(cctx, &mtproto.TLPhotosUploadContactProfilePhoto{
			Constructor: mtproto.CRC32_photos_uploadContactProfilePhoto_e14c4a71,
			UserId:      contact,
			Save:        true,
		})
	})

	if _, err = a.Invoke(ctx, &mtproto.TLPhotosUploadContactProfilePhoto{
		Constructor: mtproto.CRC32_photos_uploadContactProfilePhoto_e14c4a71,
		UserId:      contact,
		Save:        true,
	}); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if got := personalPhotoID(t, ctx, a, bSelf.ID, bSelf.AccessHash); got != 0 {
		t.Fatalf("personal_photo %d after clear", got)
	}

	fileID = randomID()
	if _, err = a.Invoke(ctx, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       jpeg,
	}); err != nil {
		t.Fatal(err)
	}
	suggested, err := a.Invoke(ctx, &mtproto.TLPhotosUploadContactProfilePhoto{
		Constructor: mtproto.CRC32_photos_uploadContactProfilePhoto_e14c4a71,
		UserId:      contact,
		Suggest:     true,
		File: mtproto.MakeTLInputFile(&mtproto.InputFile{
			Id_INT64: fileID,
			Parts:    1,
			Name:     "suggest.jpg",
		}).To_InputFile(),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := photoIDOf(t, suggested)
	if err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(ctx, peerForB, 30)
		if err != nil {
			return false, err
		}
		for _, m := range msgs {
			act := m.GetAction()
			if act.GetPredicateName() == mtproto.Predicate_messageActionSuggestProfilePhoto && act.GetPhoto().GetId() == want {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatalf("contact did not receive messageActionSuggestProfilePhoto %d: %v", want, err)
	}
}

func personalPhotoID(t *testing.T, ctx context.Context, c *Client, userID, accessHash int64) int64 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(userID, accessHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := obj.(type) {
	case *mtproto.TLUsersUserFull:
		return v.GetFullUser().GetPersonalPhoto().GetId()
	case *mtproto.Users_UserFull:
		return v.GetFullUser().GetPersonalPhoto().GetId()
	default:
		t.Fatalf("getFullUser %T", obj)
	}
	return 0
}

func photoIDOf(t *testing.T, obj mtproto.TLObject) int64 {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLPhotosPhoto:
		return v.GetPhoto().GetId()
	case *mtproto.Photos_Photo:
		return v.GetPhoto().GetId()
	default:
		t.Fatalf("photos.photo %T", obj)
	}
	return 0
}
