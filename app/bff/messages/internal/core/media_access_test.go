package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestRandomIDDedupStillChecksMediaAccess(t *testing.T) {
	set, err := catalog.Must().GetSet(0, mtproto.MakeTLInputStickerSetShortName(&mtproto.InputStickerSet{
		ShortName: catalog.ShortTwemoji,
	}).To_InputStickerSet(), 0)
	if err != nil || len(set.GetDocuments()) == 0 {
		t.Fatal(err)
	}
	doc := set.GetDocuments()[0]
	c := &MessagesCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
	}
	bad := &mtproto.TLMessagesSendMedia{
		RandomId: 42,
		Media: mtproto.MakeTLInputMediaDocument(&mtproto.InputMedia{
			Id_INPUTDOCUMENT: mtproto.MakeTLInputDocument(&mtproto.InputDocument{
				Id:         doc.GetId(),
				AccessHash: doc.GetAccessHash() + 1,
			}).To_InputDocument(),
		}).To_InputMedia(),
	}
	for i := 0; i < 2; i++ {
		_, err = c.MessagesSendMedia(bad)
		if err != mtproto.ErrMediaInvalid {
			t.Fatalf("resend %d err %v", i, err)
		}
	}
	good := &mtproto.TLMessagesSendMedia{
		RandomId: 42,
		Media: mtproto.MakeTLInputMediaDocument(&mtproto.InputMedia{
			Id_INPUTDOCUMENT: mtproto.MakeTLInputDocument(&mtproto.InputDocument{
				Id:         doc.GetId(),
				AccessHash: doc.GetAccessHash(),
			}).To_InputDocument(),
		}).To_InputMedia(),
	}
	// The hash matches, so dedupe is allowed to continue. Sending still needs
	// a peer; the access check itself must not be the failure.
	if err = c.authorizeReferencedMedia(good.GetMedia()); err != nil {
		t.Fatal(err)
	}
}
