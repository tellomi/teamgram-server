package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const archiveMuteForever int32 = 2147483647

// TestArchiveStaysOnTheServer is the owner report: folders.editPeerFolders
// folder_id=1 must stick. getDialogs folder_id=0 must not return the chat, a
// second session must see updateFolderPeers from getDifference, and a fresh
// login must still see the archive. Outgoing messages, reading history, and
// muted incoming messages stay archived. The same holds for a basic group and
// a supergroup.
func TestArchiveStaysOnTheServer(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "archive_a", "ArchiveA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "archive_b", "ArchiveB")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	setPrivacy(t, ctx, a, false, false, false)
	bPeer := importPeer(t, ctx, cfg, a, "archive_b", "ArchiveB")
	aPeer := importPeer(t, ctx, cfg, b, "archive_a", "ArchiveA")
	if _, err := b.SendText(ctx, aPeer, fmt.Sprintf("hello-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	waitFolder(t, ctx, a, "user", bPeer.GetUserId(), 0)

	other, _ := loginNamed(t, cfg, ProfileAndroid, "archive_a", "ArchiveA")
	pts, qts, date, err := other.State(ctx)
	if err != nil {
		t.Fatal(err)
	}

	archived := editPeerFolder(t, ctx, a, bPeer, 1)
	if archived.GetPts_INT32() <= 0 || archived.GetPtsCount() != 1 {
		t.Fatalf("updateFolderPeers pts=%d count=%d", archived.GetPts_INT32(), archived.GetPtsCount())
	}
	if folderOfUpdate(archived, "user", bPeer.GetUserId()) != 1 {
		t.Fatalf("archive update folder %d", folderOfUpdate(archived, "user", bPeer.GetUserId()))
	}
	assertFolder(t, ctx, a, "user", bPeer.GetUserId(), 1)

	diff, err := other.Difference(ctx, pts, qts, date)
	if err != nil {
		t.Fatal(err)
	}
	if !differenceHasFolder(diff, "user", bPeer.GetUserId(), 1) {
		t.Fatalf("second session getDifference missed updateFolderPeers: %T", diff)
	}
	assertFolder(t, ctx, other, "user", bPeer.GetUserId(), 1)

	fresh, _ := loginNamed(t, cfg, ProfileIOS, "archive_a", "ArchiveA")
	assertFolder(t, ctx, fresh, "user", bPeer.GetUserId(), 1)

	if _, err := a.SendText(ctx, bPeer, fmt.Sprintf("out-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	assertFolder(t, ctx, a, "user", bPeer.GetUserId(), 1)
	top := dialogTop(t, ctx, a, 1, "user", bPeer.GetUserId())
	if err := a.ReadHistory(ctx, bPeer, top); err != nil {
		t.Fatal(err)
	}
	assertFolder(t, ctx, a, "user", bPeer.GetUserId(), 1)

	setMute(t, ctx, a, bPeer, archiveMuteForever)
	if _, err := b.SendText(ctx, aPeer, fmt.Sprintf("muted-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	waitFolder(t, ctx, a, "user", bPeer.GetUserId(), 1)

	chatID, err := a.CreateChat(ctx, fmt.Sprintf("arch-%d", time.Now().Unix()), inputUser(bPeer.GetUserId(), bPeer.GetAccessHash()))
	if err != nil {
		t.Fatal(err)
	}
	chatPeer := inputPeerChat(chatID)
	if _, err := a.SendText(ctx, chatPeer, "group-seed", nil); err != nil {
		t.Fatal(err)
	}
	waitFolder(t, ctx, a, "chat", chatID, 0)
	editPeerFolder(t, ctx, a, chatPeer, 1)
	assertFolder(t, ctx, a, "chat", chatID, 1)
	setMute(t, ctx, a, chatPeer, archiveMuteForever)
	if _, err := b.SendText(ctx, chatPeer, fmt.Sprintf("gmuted-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	waitFolder(t, ctx, a, "chat", chatID, 1)

	chID, chHash, err := a.CreateMegagroup(ctx, fmt.Sprintf("arch-ch-%d", time.Now().Unix()))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.InviteToChannel(ctx, inputChannel(chID, chHash), inputUser(bPeer.GetUserId(), bPeer.GetAccessHash())); err != nil {
		t.Fatal(err)
	}
	chPeer := inputPeerChannel(chID, chHash)
	if _, err := a.SendText(ctx, chPeer, "channel-seed", nil); err != nil {
		t.Fatal(err)
	}
	waitFolder(t, ctx, a, "channel", chID, 0)
	editPeerFolder(t, ctx, a, chPeer, 1)
	assertFolder(t, ctx, a, "channel", chID, 1)
	setMute(t, ctx, a, chPeer, archiveMuteForever)
	var bHash int64
	err = waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		got, ok, err := b.ChatAccess(ctx, chID)
		if err != nil || !ok {
			return false, err
		}
		bHash = got
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.SendText(ctx, inputPeerChannel(chID, bHash), fmt.Sprintf("chmuted-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	waitFolder(t, ctx, a, "channel", chID, 1)
}

// TestArchiveIncomingRules covers the only official auto-unarchive: a new
// incoming message in an unmuted archived chat when keep_archived_unmuted is
// false. keep_archived_unmuted and keep_archived_folders keep it archived.
// archive_and_mute_new_noncontact_peers archives and mutes a new private chat
// from someone who is not a contact.
func TestArchiveIncomingRules(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "archive_a", "ArchiveA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "archive_b", "ArchiveB")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	bPeer := importPeer(t, ctx, cfg, a, "archive_b", "ArchiveB")
	aPeer := importPeer(t, ctx, cfg, b, "archive_a", "ArchiveA")
	if _, err := b.SendText(ctx, aPeer, fmt.Sprintf("seed-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	// An earlier test may already have archived and muted this pair.
	err := waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		return dialogFolder(t, ctx, a, 0, "user", bPeer.GetUserId()) == 0 || dialogFolder(t, ctx, a, 1, "user", bPeer.GetUserId()) == 1, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	setMute(t, ctx, a, bPeer, 0)
	editPeerFolder(t, ctx, a, bPeer, 0)
	waitFolder(t, ctx, a, "user", bPeer.GetUserId(), 0)
	setPrivacy(t, ctx, a, false, true, false)
	editPeerFolder(t, ctx, a, bPeer, 1)
	if _, err := b.SendText(ctx, aPeer, fmt.Sprintf("keep-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	waitFolder(t, ctx, a, "user", bPeer.GetUserId(), 1)

	setPrivacy(t, ctx, a, false, false, true)
	if _, err := a.Invoke(ctx, &mtproto.TLMessagesUpdateDialogFilter{
		Constructor: mtproto.CRC32_messages_updateDialogFilter,
		Id:          2,
		Filter: mtproto.MakeTLDialogFilter(&mtproto.DialogFilter{
			Id:           2,
			IncludePeers: []*mtproto.InputPeer{bPeer},
			Title_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
				Text:     "Kept",
				Entities: []*mtproto.MessageEntity{},
			}).To_TextWithEntities(),
		}).To_DialogFilter(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := b.SendText(ctx, aPeer, fmt.Sprintf("folder-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	waitFolder(t, ctx, a, "user", bPeer.GetUserId(), 1)

	setPrivacy(t, ctx, a, false, false, false)
	if _, err := b.SendText(ctx, aPeer, fmt.Sprintf("open-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	waitFolder(t, ctx, a, "user", bPeer.GetUserId(), 0)

	c, _ := loginNamed(t, cfg, ProfileIOS, "archive_c", "ArchiveC")
	setPrivacy(t, ctx, c, true, false, false)
	got := getPrivacy(t, ctx, c)
	if !got.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN() || got.GetKeepArchivedUnmuted() || got.GetKeepArchivedFolders() {
		t.Fatalf("privacy %+v", got)
	}
	if dialogFolder(t, ctx, c, 1, "user", aSelf.ID) == 1 {
		if notifyMute(t, ctx, c, aSelf.ID) != archiveMuteForever {
			t.Fatal("archived non-contact is not muted")
		}
		return
	}
	cPeer := importPeer(t, ctx, cfg, a, "archive_c", "ArchiveC")
	if _, err := a.SendText(ctx, cPeer, fmt.Sprintf("stranger-%d", time.Now().UnixNano()), nil); err != nil {
		t.Fatal(err)
	}
	waitFolder(t, ctx, c, "user", aSelf.ID, 1)
	if notifyMute(t, ctx, c, aSelf.ID) != archiveMuteForever {
		t.Fatal("new non-contact was not muted")
	}
}

func importPeer(t *testing.T, ctx context.Context, cfg Config, c *Client, name, first string) *mtproto.InputPeer {
	t.Helper()
	phone, err := cfg.Phone(name)
	if err != nil {
		t.Fatal(err)
	}
	id, hash, err := c.ImportPhone(ctx, phone.Number, first)
	if err != nil {
		t.Fatal(err)
	}
	return inputPeerUser(id, hash)
}

func assertFolder(t *testing.T, ctx context.Context, c *Client, kind string, id int64, folder int32) {
	t.Helper()
	if got := dialogFolder(t, ctx, c, 0, kind, id); folder == 0 && got != 0 || folder != 0 && got != -1 {
		t.Fatalf("main list folder for %s %d = %d, archived chat must be absent", kind, id, got)
	}
	if got := dialogFolder(t, ctx, c, 1, kind, id); got != folder && !(folder == 0 && got == -1) {
		t.Fatalf("archive list folder for %s %d = %d, want %d", kind, id, got, folder)
	}
	if folder == 1 && peerDialogFolder(t, ctx, c, kind, id) != 1 {
		t.Fatalf("getPeerDialogs %s %d is not archived", kind, id)
	}
}

func waitFolder(t *testing.T, ctx context.Context, c *Client, kind string, id int64, folder int32) {
	t.Helper()
	err := waitUntil(ctx, 400*time.Millisecond, func() (bool, error) {
		main := dialogFolder(t, ctx, c, 0, kind, id)
		arch := dialogFolder(t, ctx, c, 1, kind, id)
		switch folder {
		case 0:
			return main == 0 && arch == -1, nil
		case 1:
			return main == -1 && arch == 1, nil
		default:
			return main == -1 && arch == -1, nil
		}
	})
	if err != nil {
		t.Fatalf("folder %s %d want %d: %v", kind, id, folder, err)
	}
}

func dialogFolder(t *testing.T, ctx context.Context, c *Client, folder int32, kind string, id int64) int32 {
	t.Helper()
	req := &mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
	}
	if folder != 0 {
		req.FolderId = &wrapperspb.Int32Value{Value: folder}
	}
	obj, err := c.Invoke(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dialogsOf(obj) {
		if d == nil || d.GetPeer() == nil || peerKindID(d.GetPeer()) != kind+"/"+fmt.Sprint(id) {
			continue
		}
		if d.GetFolderId() == nil {
			return 0
		}
		return d.GetFolderId().GetValue()
	}
	return -1
}

func peerDialogFolder(t *testing.T, ctx context.Context, c *Client, kind string, id int64) int32 {
	t.Helper()
	var peer *mtproto.InputPeer
	switch kind {
	case "user":
		peer = inputPeerUser(id, userAccess(t, ctx, c, id))
	case "chat":
		peer = inputPeerChat(id)
	case "channel":
		peer = inputPeerChannel(id, channelAccess(t, ctx, c, id))
	}
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetPeerDialogs{
		Constructor: mtproto.CRC32_messages_getPeerDialogs,
		Peers: []*mtproto.InputDialogPeer{
			mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{Peer: peer}).To_InputDialogPeer(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dialogsOf(obj) {
		if d == nil || d.GetPeer() == nil || peerKindID(d.GetPeer()) != kind+"/"+fmt.Sprint(id) {
			continue
		}
		if d.GetFolderId() == nil {
			return 0
		}
		return d.GetFolderId().GetValue()
	}
	return -1
}

func dialogTop(t *testing.T, ctx context.Context, c *Client, folder int32, kind string, id int64) int32 {
	t.Helper()
	req := &mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
		FolderId:    &wrapperspb.Int32Value{Value: folder},
	}
	obj, err := c.Invoke(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range dialogsOf(obj) {
		if d != nil && d.GetPeer() != nil && peerKindID(d.GetPeer()) == kind+"/"+fmt.Sprint(id) {
			return d.GetTopMessage()
		}
	}
	t.Fatalf("no dialog %s %d in folder %d", kind, id, folder)
	return 0
}

func dialogsOf(obj mtproto.TLObject) []*mtproto.Dialog {
	switch d := obj.(type) {
	case *mtproto.TLMessagesDialogs:
		return d.GetDialogs()
	case *mtproto.TLMessagesDialogsSlice:
		return d.GetDialogs()
	case *mtproto.TLMessagesPeerDialogs:
		return d.GetDialogs()
	default:
		return nil
	}
}

func peerKindID(peer *mtproto.Peer) string {
	switch peer.GetPredicateName() {
	case mtproto.Predicate_peerUser:
		return "user/" + fmt.Sprint(peer.GetUserId())
	case mtproto.Predicate_peerChat:
		return "chat/" + fmt.Sprint(peer.GetChatId())
	case mtproto.Predicate_peerChannel:
		return "channel/" + fmt.Sprint(peer.GetChannelId())
	default:
		return ""
	}
}

func channelAccess(t *testing.T, ctx context.Context, c *Client, id int64) int64 {
	t.Helper()
	if hash, ok, err := c.ChatAccess(ctx, id); err == nil && ok && hash != 0 {
		return hash
	}
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
		FolderId:    &wrapperspb.Int32Value{Value: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, chat := range chatsOf(obj) {
		if chat != nil && chat.GetId() == id {
			if hash := chatAccessHash(chat); hash != 0 {
				return hash
			}
		}
	}
	t.Fatalf("channel access %d", id)
	return 0
}

func userAccess(t *testing.T, ctx context.Context, c *Client, id int64) int64 {
	t.Helper()
	page, err := c.dialogPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range page.Users {
		if u != nil && u.GetId() == id && u.GetAccessHash() != nil {
			return u.GetAccessHash().GetValue()
		}
	}
	obj, err := c.Invoke(ctx, &mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
		FolderId:    &wrapperspb.Int32Value{Value: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, u := range usersOf(obj) {
		if u != nil && u.GetId() == id && u.GetAccessHash() != nil {
			return u.GetAccessHash().GetValue()
		}
	}
	t.Fatalf("no access hash for user %d", id)
	return 0
}

func usersOf(obj mtproto.TLObject) []*mtproto.User {
	switch d := obj.(type) {
	case *mtproto.TLMessagesDialogs:
		return d.GetUsers()
	case *mtproto.TLMessagesDialogsSlice:
		return d.GetUsers()
	case *mtproto.TLMessagesPeerDialogs:
		return d.GetUsers()
	default:
		return nil
	}
}

func editPeerFolder(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, folder int32) *mtproto.Update {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLFoldersEditPeerFolders{
		Constructor: mtproto.CRC32_folders_editPeerFolders,
		FolderPeers: []*mtproto.InputFolderPeer{
			mtproto.MakeTLInputFolderPeer(&mtproto.InputFolderPeer{
				Peer:     peer,
				FolderId: folder,
			}).To_InputFolderPeer(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, upd := range updatesOf(obj) {
		if upd.GetPredicateName() == mtproto.Predicate_updateFolderPeers {
			return upd
		}
	}
	t.Fatalf("folders.editPeerFolders returned %T without updateFolderPeers", obj)
	return nil
}

func updatesOf(obj mtproto.TLObject) []*mtproto.Update {
	switch v := obj.(type) {
	case *mtproto.TLUpdates:
		return v.GetUpdates()
	case *mtproto.TLUpdatesCombined:
		return v.GetUpdates()
	case *mtproto.TLUpdateShort:
		if v.GetUpdate() != nil {
			return []*mtproto.Update{v.GetUpdate()}
		}
	}
	return nil
}

func folderOfUpdate(upd *mtproto.Update, kind string, id int64) int32 {
	for _, fp := range upd.GetFolderPeers() {
		if fp != nil && fp.GetPeer() != nil && peerKindID(fp.GetPeer()) == kind+"/"+fmt.Sprint(id) {
			return fp.GetFolderId()
		}
	}
	return -1
}

func differenceHasFolder(obj mtproto.TLObject, kind string, id int64, folder int32) bool {
	var ups []*mtproto.Update
	switch v := obj.(type) {
	case *mtproto.TLUpdatesDifference:
		ups = v.GetOtherUpdates()
	case *mtproto.TLUpdatesDifferenceSlice:
		ups = v.GetOtherUpdates()
	}
	for _, upd := range ups {
		if upd.GetPredicateName() == mtproto.Predicate_updateFolderPeers && folderOfUpdate(upd, kind, id) == folder && upd.GetPts_INT32() > 0 {
			return true
		}
	}
	return false
}

func setMute(t *testing.T, ctx context.Context, c *Client, peer *mtproto.InputPeer, until int32) {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountUpdateNotifySettings{
		Constructor: mtproto.CRC32_account_updateNotifySettings,
		Peer: mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
			Peer: peer,
		}).To_InputNotifyPeer(),
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			MuteUntil: &wrapperspb.Int32Value{Value: until},
		}).To_InputPeerNotifySettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := obj.(*mtproto.TLBoolTrue); !ok {
		t.Fatalf("updateNotifySettings %T", obj)
	}
}

func notifyMute(t *testing.T, ctx context.Context, c *Client, userID int64) int32 {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountGetNotifySettings{
		Constructor: mtproto.CRC32_account_getNotifySettings,
		Peer: mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
			Peer: inputPeerUser(userID, userAccess(t, ctx, c, userID)),
		}).To_InputNotifyPeer(),
	})
	if err != nil {
		t.Fatal(err)
	}
	settings, ok := obj.(*mtproto.PeerNotifySettings)
	if !ok {
		if tl, ok := obj.(*mtproto.TLPeerNotifySettings); ok {
			return tl.GetMuteUntil().GetValue()
		}
		t.Fatalf("notify settings %T", obj)
	}
	return settings.GetMuteUntil().GetValue()
}

func setPrivacy(t *testing.T, ctx context.Context, c *Client, archiveNew, keepUnmuted, keepFolders bool) {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountSetGlobalPrivacySettings{
		Constructor: mtproto.CRC32_account_setGlobalPrivacySettings,
		Settings: mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
			ArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN: archiveNew,
			KeepArchivedUnmuted:                          keepUnmuted,
			KeepArchivedFolders:                          keepFolders,
		}).To_GlobalPrivacySettings(),
	})
	if err != nil {
		t.Fatal(err)
	}
	got := privacyOf(t, obj)
	if got.GetKeepArchivedUnmuted() != keepUnmuted || got.GetKeepArchivedFolders() != keepFolders {
		t.Fatalf("set privacy %+v", got)
	}
	if archiveNew != (got.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN() || mtproto.FromBool(got.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOL())) {
		t.Fatalf("archive_and_mute %+v", got)
	}
}

func getPrivacy(t *testing.T, ctx context.Context, c *Client) *mtproto.GlobalPrivacySettings {
	t.Helper()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountGetGlobalPrivacySettings{
		Constructor: mtproto.CRC32_account_getGlobalPrivacySettings,
	})
	if err != nil {
		t.Fatal(err)
	}
	return privacyOf(t, obj)
}

func privacyOf(t *testing.T, obj mtproto.TLObject) *mtproto.GlobalPrivacySettings {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLGlobalPrivacySettings:
		return v.To_GlobalPrivacySettings()
	case *mtproto.GlobalPrivacySettings:
		return v
	default:
		t.Fatalf("privacy %T", obj)
		return nil
	}
}
