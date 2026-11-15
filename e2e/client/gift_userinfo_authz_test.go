package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestGiftCollectionAndUserInfoAuthz covers ISSUE-S-GIFTCOLL-AUTHZ-001 (row 346)
// and ISSUE-S-HELP-USERINFO-AUTHZ-001 (row 347).
// Phones 9996610481–484, code 11111, layer 232.
func TestGiftCollectionAndUserInfoAuthz(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	owner, ownerSelf := dialPhone(t, ctx, cfg, "9996610481", "GiftOwner")
	stranger, _ := dialPhone(t, ctx, cfg, "9996610482", "GiftStranger")
	ownerID, ownerHash, err := stranger.ImportPhone(ctx, "9996610481", "GiftOwner")
	if err != nil {
		t.Fatal(err)
	}
	if ownerID == 0 {
		ownerID, ownerHash = ownerSelf.ID, ownerSelf.AccessHash
	}

	madeObj, err := owner.Invoke(ctx, &mtproto.TLPaymentsCreateStarGiftCollection{
		Constructor: mtproto.CRC32_payments_createStarGiftCollection,
		Peer:        inputPeerSelf(),
		Title:       "Pins",
		Stargift:    []*mtproto.InputSavedStarGift{},
	})
	if err != nil {
		t.Fatal(err)
	}
	made, ok := madeObj.(*mtproto.TLStarGiftCollection)
	if !ok || made.GetCollectionId() == 0 || made.GetTitle() != "Pins" {
		t.Fatalf("create %#v", madeObj)
	}
	if _, err = stranger.Invoke(ctx, &mtproto.TLPaymentsCreateStarGiftCollection{
		Constructor: mtproto.CRC32_payments_createStarGiftCollection,
		Peer:        inputPeerUser(ownerID, ownerHash),
		Title:       "Stolen",
		Stargift:    []*mtproto.InputSavedStarGift{},
	}); err == nil {
		t.Fatal("stranger created a collection on another profile")
	} else {
		wantRPC(t, err, 400, "PEER_ID_INVALID")
	}
	if _, err = stranger.Invoke(ctx, &mtproto.TLPaymentsUpdateStarGiftCollection{
		Constructor:  mtproto.CRC32_payments_updateStarGiftCollection,
		Peer:         inputPeerUser(ownerID, ownerHash),
		CollectionId: made.GetCollectionId(),
		Title:        strFlag("Hijack"),
	}); err == nil {
		t.Fatal("stranger renamed a collection")
	} else {
		wantRPC(t, err, 400, "PEER_ID_INVALID")
	}
	if _, err = stranger.Invoke(ctx, &mtproto.TLPaymentsDeleteStarGiftCollection{
		Constructor:  mtproto.CRC32_payments_deleteStarGiftCollection,
		Peer:         inputPeerUser(ownerID, ownerHash),
		CollectionId: made.GetCollectionId(),
	}); err == nil {
		t.Fatal("stranger deleted a collection")
	} else {
		wantRPC(t, err, 400, "PEER_ID_INVALID")
	}
	listedObj, err := owner.Invoke(ctx, &mtproto.TLPaymentsGetStarGiftCollections{
		Constructor: mtproto.CRC32_payments_getStarGiftCollections,
		Peer:        inputPeerSelf(),
	})
	if err != nil {
		t.Fatal(err)
	}
	listed, ok := listedObj.(*mtproto.TLPaymentsStarGiftCollections)
	if !ok || len(listed.GetCollections()) == 0 || listed.GetCollections()[0].GetTitle() != "Pins" {
		t.Fatalf("owner list %#v", listedObj)
	}

	editor, _ := dialPhone(t, ctx, cfg, "9996610483", "NoteEditor")
	subject, subjectSelf := dialPhone(t, ctx, cfg, "9996610484", "NoteSubject")
	subjectID, subjectHash, err := editor.ImportPhone(ctx, "9996610484", "NoteSubject")
	if err != nil {
		t.Fatal(err)
	}
	if subjectID == 0 {
		subjectID, subjectHash = subjectSelf.ID, subjectSelf.AccessHash
	}
	noteUser := inputUser(subjectID, subjectHash)
	if _, err = editor.Invoke(ctx, &mtproto.TLHelpEditUserInfo{
		Constructor: mtproto.CRC32_help_editUserInfo,
		UserId:      noteUser,
		Message:     "secret note",
		Entities:    []*mtproto.MessageEntity{},
	}); err == nil {
		t.Fatal("non-support user wrote a support note")
	} else {
		wantRPC(t, err, 403, "USER_INVALID")
	}
	if _, err = subject.Invoke(ctx, &mtproto.TLHelpGetUserInfo{
		Constructor: mtproto.CRC32_help_getUserInfo,
		UserId:      inputUserSelf(),
	}); err == nil {
		t.Fatal("subject read a support note")
	} else {
		wantRPC(t, err, 403, "USER_INVALID")
	}
}
