package e2e

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestHideReadMarksBlocksOutboxReadDate follows messages.getOutboxReadDate.
// After Bob turns on hide_read_marks, Alice's lookup returns
// USER_PRIVACY_RESTRICTED. Bob also cannot look up Alice, which is
// YOUR_PRIVACY_RESTRICTED unless the caller has Premium.
func TestHideReadMarksBlocksOutboxReadDate(t *testing.T) {
	a, b, _, bSelf := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	peerForA := inputPeerUser(bSelf.ID, bSelf.AccessHash)
	peerForB := inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)

	prev := getPrivacy(t, ctx, b)
	t.Cleanup(func() {
		cctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		setHideReadMarks(t, cctx, b, prev, false)
	})

	text := fmt.Sprintf("read-date-%d", time.Now().UnixNano())
	if _, err := a.SendText(ctx, peerForA, text, nil); err != nil {
		t.Fatal(err)
	}
	var bobID int32
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(ctx, peerForB, 20)
		if err != nil {
			return false, err
		}
		bobID = idOfText(msgs, text)
		return bobID != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := b.ReadHistory(ctx, peerForB, bobID); err != nil {
		t.Fatal(err)
	}
	var aliceID int32
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := a.History(ctx, peerForA, 20)
		if err != nil {
			return false, err
		}
		aliceID = idOfText(msgs, text)
		return aliceID != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := outboxReadDate(ctx, a, peerForA, aliceID); err != nil {
		t.Fatalf("read date before the privacy setting: %v", err)
	}

	setHideReadMarks(t, ctx, b, prev, true)
	full, err := a.Invoke(ctx, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(bSelf.ID, bSelf.AccessHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !fullUserOf(t, full).GetReadDatesPrivate() {
		t.Fatal("userFull.read_dates_private was not set")
	}
	_, err = outboxReadDate(ctx, a, peerForA, aliceID)
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 403 || rpc.Message != "USER_PRIVACY_RESTRICTED" {
		t.Fatalf("alice looking up bob: %v", err)
	}

	back := fmt.Sprintf("read-date-back-%d", time.Now().UnixNano())
	if _, err = b.SendText(ctx, peerForB, back, nil); err != nil {
		t.Fatal(err)
	}
	var own int32
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := a.History(ctx, peerForA, 20)
		if err != nil {
			return false, err
		}
		own = idOfText(msgs, back)
		return own != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err = a.ReadHistory(ctx, peerForA, own); err != nil {
		t.Fatal(err)
	}
	var bobMsg int32
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		msgs, err := b.History(ctx, peerForB, 20)
		if err != nil {
			return false, err
		}
		bobMsg = idOfText(msgs, back)
		return bobMsg != 0, nil
	}); err != nil {
		t.Fatal(err)
	}
	_, err = outboxReadDate(ctx, b, peerForB, bobMsg)
	if rpc, ok := asRPCError(err); !ok || rpc.Code != 403 || rpc.Message != "YOUR_PRIVACY_RESTRICTED" {
		t.Fatalf("bob looking up alice: %v", err)
	}
}

func outboxReadDate(ctx context.Context, c *Client, peer *mtproto.InputPeer, msgID int32) (mtproto.TLObject, error) {
	return c.Invoke(ctx, &mtproto.TLMessagesGetOutboxReadDate{
		Constructor: mtproto.CRC32_messages_getOutboxReadDate,
		Peer:        peer,
		MsgId:       msgID,
	})
}

func setHideReadMarks(t *testing.T, ctx context.Context, c *Client, base *mtproto.GlobalPrivacySettings, hide bool) {
	t.Helper()
	settings := mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
		ArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN: base.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN(),
		KeepArchivedUnmuted:                          base.GetKeepArchivedUnmuted(),
		KeepArchivedFolders:                          base.GetKeepArchivedFolders(),
		HideReadMarks:                                hide,
		NewNoncontactPeersRequirePremium:             base.GetNewNoncontactPeersRequirePremium(),
	}).To_GlobalPrivacySettings()
	obj, err := c.Invoke(ctx, &mtproto.TLAccountSetGlobalPrivacySettings{
		Constructor: mtproto.CRC32_account_setGlobalPrivacySettings,
		Settings:    settings,
	})
	if err != nil {
		t.Fatal(err)
	}
	if privacyOf(t, obj).GetHideReadMarks() != hide {
		t.Fatalf("hide_read_marks %+v", privacyOf(t, obj))
	}
}

func fullUserOf(t *testing.T, obj mtproto.TLObject) *mtproto.UserFull {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLUsersUserFull:
		return v.GetFullUser()
	case *mtproto.Users_UserFull:
		return v.GetFullUser()
	default:
		t.Fatalf("getFullUser %T", obj)
	}
	return nil
}
