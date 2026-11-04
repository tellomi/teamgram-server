package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestMegagroupSettingsAreNotEnterprise covers the channel branches that used
// to return ENTERPRISE_IS_BLOCKED while a user edited the description, saved a
// draft, muted the chat, or cleared their own history.
func TestMegagroupSettingsAreNotEnterprise(t *testing.T) {
	a, _, aSelf, _ := sharedPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	id, hash, err := a.CreateMegagroup(ctx, "e2e-settings")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerChannel(id, hash)
	if _, err := a.Invoke(ctx, &mtproto.TLMessagesEditChatAbout{
		Constructor: mtproto.CRC32_messages_editChatAbout,
		Peer:        peer,
		About:       "群简介",
	}); err != nil {
		t.Fatal(err)
	}
	fullObj, err := a.Invoke(ctx, &mtproto.TLChannelsGetFullChannel{
		Constructor: mtproto.CRC32_channels_getFullChannel,
		Channel:     inputChannel(id, hash),
	})
	if err != nil {
		t.Fatal(err)
	}
	if chatFullOf(t, fullObj).GetAbout() != "群简介" {
		t.Fatalf("about %q", chatFullOf(t, fullObj).GetAbout())
	}

	if _, err := a.Invoke(ctx, &mtproto.TLMessagesSaveDraft{
		Constructor: mtproto.CRC32_messages_saveDraft_ad0fa15c,
		Peer:        peer,
		Message:     "还没发出去",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Invoke(ctx, &mtproto.TLAccountUpdateNotifySettings{
		Constructor: mtproto.CRC32_account_updateNotifySettings,
		Peer: mtproto.MakeTLInputNotifyPeer(&mtproto.InputNotifyPeer{
			Peer: peer,
		}).To_InputNotifyPeer(),
		Settings: mtproto.MakeTLInputPeerNotifySettings(&mtproto.InputPeerNotifySettings{
			Silent:    mtproto.BoolTrue,
			MuteUntil: &wrapperspb.Int32Value{Value: int32(time.Now().Add(time.Hour).Unix())},
		}).To_InputPeerNotifySettings(),
	}); err != nil {
		t.Fatal(err)
	}

	histObj, err := a.Invoke(ctx, &mtproto.TLMessagesDeleteHistory{
		Constructor: mtproto.CRC32_messages_deleteHistory,
		JustClear:   true,
		Peer:        peer,
		MaxId:       0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if ptsCount(t, histObj) != 0 {
		t.Fatalf("deleteHistory moved pts: %T %+v", histObj, histObj)
	}

	prem, err := a.Invoke(ctx, &mtproto.TLUsersGetIsPremiumRequiredToContact{
		Constructor: mtproto.CRC32_users_getIsPremiumRequiredToContact,
		Id:          []*mtproto.InputUser{inputUser(aSelf.ID, aSelf.AccessHash)},
	})
	if err != nil {
		t.Fatal(err)
	}
	vec, ok := prem.(*tlVector)
	if !ok || vec.Count != 1 {
		t.Fatalf("premium required %T %+v", prem, prem)
	}
}

func chatFullOf(t *testing.T, obj mtproto.TLObject) *mtproto.ChatFull {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesChatFull:
		return v.To_Messages_ChatFull().GetFullChat()
	case *mtproto.Messages_ChatFull:
		return v.GetFullChat()
	default:
		t.Fatalf("full %T", obj)
		return nil
	}
}

func ptsCount(t *testing.T, obj mtproto.TLObject) int32 {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLMessagesAffectedHistory:
		return v.To_Messages_AffectedHistory().GetPtsCount()
	case *mtproto.Messages_AffectedHistory:
		return v.GetPtsCount()
	default:
		t.Fatalf("affected history %T", obj)
		return -1
	}
}
