package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestStickerListSyncPush is ISSUE-S-STICKER-SYNC-PUSH-001.
// Two sessions of one account. saveGif pushes updateSavedGifs to the other
// session and not to the caller. faveSticker of a sticker from a pack this
// account just created pushes updateFavedStickers the same way.
// Phone 9996610794, code 11111.
func TestStickerListSyncPush(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	const phone = "9996610794"
	a, self := dialPhone(t, ctx, cfg, phone, "StickerSync")
	b, _ := dialPhone(t, ctx, cfg, phone, "StickerSync")
	if _, _, _, err := a.State(ctx); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}

	page := savedGifs(t, ctx, a)
	if len(page.GetGifs()) < 2 {
		t.Fatalf("saved gifs %d", len(page.GetGifs()))
	}
	// Saved rows are listed first, so the last id is not the first stored row.
	// Saving it changes the order even when this phone already has saved GIFs.
	doc := page.GetGifs()[len(page.GetGifs())-1]
	if doc.GetId() == page.GetGifs()[0].GetId() {
		doc = page.GetGifs()[1]
	}
	gif := mtproto.MakeTLInputDocument(&mtproto.InputDocument{
		Id:            doc.GetId(),
		AccessHash:    doc.GetAccessHash(),
		FileReference: append([]byte(nil), doc.GetFileReference()...),
	}).To_InputDocument()
	if err := expectOtherSession(ctx, a, b, mtproto.Predicate_updateSavedGifs, func() error {
		_, err := a.Invoke(ctx, &mtproto.TLMessagesSaveGif{
			Constructor: mtproto.CRC32_messages_saveGif,
			Id:          gif,
			Unsave:      mtproto.BoolFalse,
		})
		return err
	}); err != nil {
		t.Fatalf("other session missed updateSavedGifs: %v; other=%s self=%s", err, pushedSummary(b.Pushed()), pushedSummary(a.Pushed()))
	}

	uploaded := uploadStickerDocument(t, a, "image/webp", "sticker.webp", tinyStickerWEBP)
	created := createStickerSet(t, a, self, "", []*mtproto.InputStickerSetItem{stickerItem(uploaded, "🫥")})
	if len(created.GetDocuments()) != 1 || created.GetDocuments()[0].GetId() == 0 {
		t.Fatalf("created pack %#v", created.GetSet())
	}
	stickerDoc := created.GetDocuments()[0]
	sticker := mtproto.MakeTLInputDocument(&mtproto.InputDocument{
		Id:            stickerDoc.GetId(),
		AccessHash:    stickerDoc.GetAccessHash(),
		FileReference: append([]byte(nil), stickerDoc.GetFileReference()...),
	}).To_InputDocument()
	if err := expectOtherSession(ctx, a, b, mtproto.Predicate_updateFavedStickers, func() error {
		_, err := a.Invoke(ctx, &mtproto.TLMessagesFaveSticker{
			Constructor: mtproto.CRC32_messages_faveSticker,
			Id:          sticker,
			Unfave:      mtproto.BoolFalse,
		})
		return err
	}); err != nil {
		t.Fatalf("other session missed updateFavedStickers: %v; other=%s self=%s", err, pushedSummary(b.Pushed()), pushedSummary(a.Pushed()))
	}
}

// expectOtherSession runs mutate once. The other session must be pushed name.
// The caller must not. Marks are taken before the write so a push that arrives
// during the RPC still counts. Saving again would be a no-op and would not push.
func expectOtherSession(ctx context.Context, self, other *Client, name string, mutate func() error) error {
	selfMark := len(self.Pushed())
	otherMark := len(other.Pushed())
	if err := mutate(); err != nil {
		return err
	}
	deadline, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	return waitUntil(deadline, 200*time.Millisecond, func() (bool, error) {
		if pushedName(self.Pushed()[selfMark:], name) {
			return false, errPushedToSelf(name)
		}
		return pushedName(other.Pushed()[otherMark:], name), nil
	})
}

type selfPushError string

func (e selfPushError) Error() string { return string(e) }

func errPushedToSelf(name string) error {
	return selfPushError("caller was pushed " + name)
}

func pushedName(objs []mtproto.TLObject, name string) bool {
	for _, obj := range objs {
		for _, u := range updatesOf(obj) {
			if u.GetPredicateName() == name {
				return true
			}
		}
	}
	return false
}
