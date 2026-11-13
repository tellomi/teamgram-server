//go:build reliability

package e2e

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestReliabilityPersistenceAcrossRestart stores a private-chat history TTL,
// an archived dialog, and a story expiry, restarts every teamgram process,
// and reads them back. The datastores stay up. A story whose expire_date is
// already past is absent after that restart.
func TestReliabilityPersistenceAcrossRestart(t *testing.T) {
	// Restarts every teamgram process. Not parallel with the other tests.
	s := useReliabilityStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	alicePhone := reliabilityPhone("0311")
	bobPhone := reliabilityPhone("0312")
	alice, aliceSelf := s.login(t, ProfileIOS, alicePhone, "PersistA")
	bob, bobSelf := s.login(t, ProfileAndroid, bobPhone, "PersistB")
	bobID, bobHash, err := alice.ImportPhone(ctx, bobPhone.Number, "PersistB")
	if err != nil {
		t.Fatal(err)
	}
	if bobID != bobSelf.ID {
		t.Fatalf("imported bob %d, login %d", bobID, bobSelf.ID)
	}
	peer := inputPeerUser(bobID, bobHash)
	if _, err = alice.SendText(ctx, peer, "persist-ttl", nil); err != nil {
		t.Fatal(err)
	}
	// messages.sendMessage returns when Inbox-T accepts the record. The
	// dialog row, including top_message, is written by the inbox consumer
	// after that ack. folders.editPeerFolders answers PEER_ID_INVALID until
	// that row is visible. Wait for it, then keep the same TTL and archive
	// checks. This does not call messages.getPeerDialogs.
	waitForStoredUserDialog(t, ctx, alice, bobSelf.ID)
	invokeOK(t, alice, &mtproto.TLMessagesSetHistoryTTL{
		Constructor: mtproto.CRC32_messages_setHistoryTTL,
		Peer:        peer,
		Period:      86400,
	})
	invokeOK(t, alice, &mtproto.TLFoldersEditPeerFolders{
		Constructor: mtproto.CRC32_folders_editPeerFolders,
		FolderPeers: []*mtproto.InputFolderPeer{
			mtproto.MakeTLInputFolderPeer(&mtproto.InputFolderPeer{
				Peer:     peer,
				FolderId: 1,
			}).To_InputFolderPeer(),
		},
	})

	photo := uploadJPEG(t, ctx, alice, solidJPEG(t, 40))
	up := invokeOK(t, alice, &mtproto.TLStoriesSendStory{
		Constructor: mtproto.CRC32_stories_sendStory_8f9e6898,
		Peer:        inputPeerSelf(),
		Media: mtproto.MakeTLInputMediaPhoto(&mtproto.InputMedia{
			Id_INPUTPHOTO: mtproto.MakeTLInputPhoto(&mtproto.InputPhoto{
				Id:            photo.GetId(),
				AccessHash:    photo.GetAccessHash(),
				FileReference: photo.GetFileReference(),
			}).To_InputPhoto(),
		}).To_InputMedia(),
		PrivacyRules: []*mtproto.InputPrivacyRule{
			mtproto.MakeTLInputPrivacyValueAllowAll(nil).To_InputPrivacyRule(),
		},
		RandomId: randomID(),
		Period:   &wrapperspb.Int32Value{Value: 86400},
	})
	storyID := storyIDFromUpdates(t, up)
	expire := storyExpire(t, ctx, alice, storyID)
	if expire < int32(time.Now().Unix())+86000 || expire > int32(time.Now().Unix())+86800 {
		t.Fatalf("story %d expire_date %d", storyID, expire)
	}
	if got := dialogTTL(t, ctx, alice, bobSelf.ID); got != 86400 {
		t.Fatalf("ttl_period %d", got)
	}
	if !dialogArchived(t, ctx, alice, bobSelf.ID) {
		t.Fatal("peer is not archived")
	}

	aliceSess, ok := alice.Session()
	if !ok {
		t.Fatal("missing alice session")
	}
	alice.Close()
	bob.Close()
	s.restartProcesses(t)

	alice, err = DialSession(ctx, s.cfg, ProfileIOS, aliceSess)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(alice.Close)
	if _, _, _, err = alice.State(ctx); err != nil {
		t.Fatal(err)
	}
	if got := dialogTTL(t, ctx, alice, bobSelf.ID); got != 86400 {
		t.Fatalf("ttl_period after restart %d", got)
	}
	if !dialogArchived(t, ctx, alice, bobSelf.ID) {
		t.Fatal("archive did not survive restart")
	}
	if got := storyExpire(t, ctx, alice, storyID); got != expire {
		t.Fatalf("story expire_date %d after restart, want %d", got, expire)
	}

	s.execSQL(t, "UPDATE stories SET expire_date = UNIX_TIMESTAMP() - 30 WHERE owner_kind = 1 AND owner_id = ? AND story_id = ?", aliceSelf.ID, storyID)
	alice.Close()
	s.restartProcesses(t)
	alice, err = DialSession(ctx, s.cfg, ProfileIOS, aliceSess)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(alice.Close)
	if _, found := findStory(t, ctx, alice, storyID); found {
		t.Fatal("expired story is still returned after restart")
	}
}

func waitForStoredUserDialog(t *testing.T, ctx context.Context, c *Client, userID int64) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		if d := findDialog(t, ctx, c, userID, 0); d != nil && d.GetTopMessage() > 0 {
			return
		}
		if ctx.Err() != nil || !time.Now().Before(deadline) {
			t.Fatalf("dialog for %d was not stored with a message id", userID)
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			t.Fatalf("dialog for %d was not stored with a message id: %v", userID, ctx.Err())
		case <-timer.C:
		}
	}
}

func dialogTTL(t *testing.T, ctx context.Context, c *Client, userID int64) int32 {
	t.Helper()
	d := findDialog(t, ctx, c, userID, 0)
	if d == nil {
		d = findDialog(t, ctx, c, userID, 1)
	}
	if d == nil {
		t.Fatalf("no dialog for %d", userID)
	}
	if d.GetTtlPeriod() == nil {
		return 0
	}
	return d.GetTtlPeriod().GetValue()
}

func dialogArchived(t *testing.T, ctx context.Context, c *Client, userID int64) bool {
	t.Helper()
	main := findDialog(t, ctx, c, userID, 0)
	archived := findDialog(t, ctx, c, userID, 1)
	if archived == nil {
		return false
	}
	if main != nil && (main.GetFolderId() == nil || main.GetFolderId().GetValue() != 1) {
		return false
	}
	return archived.GetFolderId() != nil && archived.GetFolderId().GetValue() == 1
}

func dialogFor(t *testing.T, ctx context.Context, c *Client, userID int64, folder int32) *mtproto.Dialog {
	t.Helper()
	d := findDialog(t, ctx, c, userID, folder)
	if d == nil {
		t.Fatalf("no dialog for %d in folder %d", userID, folder)
	}
	return d
}

func findDialog(t *testing.T, ctx context.Context, c *Client, userID int64, folder int32) *mtproto.Dialog {
	t.Helper()
	req := &mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
	}
	if folder != 0 {
		req.FolderId = &wrapperspb.Int32Value{Value: folder}
	}
	res, err := c.Invoke(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	var dialogs []*mtproto.Dialog
	switch d := res.(type) {
	case *mtproto.TLMessagesDialogs:
		dialogs = d.GetDialogs()
	case *mtproto.TLMessagesDialogsSlice:
		dialogs = d.GetDialogs()
	default:
		t.Fatalf("messages.getDialogs returned %T", res)
	}
	for _, d := range dialogs {
		if d.GetPeer() != nil && d.GetPeer().GetUserId() == userID {
			return d
		}
	}
	return nil
}

func storyExpire(t *testing.T, ctx context.Context, c *Client, id int32) int32 {
	t.Helper()
	item, ok := findStory(t, ctx, c, id)
	if !ok {
		t.Fatalf("story %d is not active", id)
	}
	return item.GetExpireDate()
}

func findStory(t *testing.T, ctx context.Context, c *Client, id int32) (*mtproto.StoryItem, bool) {
	t.Helper()
	res, err := c.Invoke(ctx, &mtproto.TLStoriesGetPeerStories{
		Constructor: mtproto.CRC32_stories_getPeerStories,
		Peer:        inputPeerSelf(),
	})
	if err != nil {
		t.Fatal(err)
	}
	peer, ok := res.(*mtproto.TLStoriesPeerStories)
	if !ok || peer.GetData2() == nil || peer.GetData2().GetStories() == nil {
		t.Fatalf("stories.getPeerStories returned %T", res)
	}
	for _, item := range peer.GetData2().GetStories().GetStories() {
		if item.GetId() == id {
			return item, true
		}
	}
	return nil, false
}

func storyIDFromUpdates(t *testing.T, obj mtproto.TLObject) int32 {
	t.Helper()
	var updates []*mtproto.Update
	switch u := obj.(type) {
	case *mtproto.TLUpdates:
		updates = u.GetUpdates()
	case *mtproto.TLUpdatesCombined:
		updates = u.GetUpdates()
	default:
		t.Fatalf("stories.sendStory returned %T", obj)
	}
	for _, up := range updates {
		if up.GetPredicateName() == mtproto.Predicate_updateStoryID && up.GetId_INT32() != 0 {
			return up.GetId_INT32()
		}
	}
	t.Fatal("stories.sendStory did not return updateStoryID")
	return 0
}

func solidJPEG(t *testing.T, mark byte) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 320, 240))
	for y := 0; y < 240; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, color.RGBA{R: mark, G: uint8(x), B: uint8(y), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func uploadJPEG(t *testing.T, ctx context.Context, c *Client, payload []byte) *mtproto.Photo {
	t.Helper()
	fileID := randomID()
	if _, err := c.Invoke(ctx, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       payload,
	}); err != nil {
		t.Fatal(err)
	}
	res, err := c.Invoke(ctx, &mtproto.TLMessagesUploadMedia{
		Constructor: mtproto.CRC32_messages_uploadMedia_14967978,
		Peer:        inputPeerSelf(),
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID,
				Parts:    1,
				Name:     "reliability.jpg",
			}).To_InputFile(),
		}).To_InputMedia(),
	})
	if err != nil {
		t.Fatal(err)
	}
	media, ok := res.(*mtproto.TLMessageMediaPhoto)
	if !ok || media.GetPhoto_FLAGPHOTO() == nil || media.GetPhoto_FLAGPHOTO().GetId() == 0 {
		t.Fatalf("messages.uploadMedia returned %T", res)
	}
	return media.GetPhoto_FLAGPHOTO()
}
