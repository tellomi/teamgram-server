package dao

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

func TestWallpaperOverriddenForBoth(t *testing.T) {
	ctx := context.Background()
	paper := mtproto.MakeTLWallPaperNoFile(&mtproto.WallPaper{Id: 88001}).To_WallPaper()
	action := mtproto.MakeTLMessageActionSetChatWallPaper(&mtproto.MessageAction{
		ForBoth:   true,
		Wallpaper: paper,
	}).To_MessageAction()

	sender := &wallDialog{}
	(&Dao{DialogClient: sender}).stampChatDecor(ctx, 1, mtproto.PEER_USER, 2, &mtproto.Message{
		Out:    true,
		FromId: mtproto.MakePeerUser(1),
		Action: action,
	})
	if sender.got == nil || sender.got.WallpaperOverridden || sender.got.WallpaperId != 88001 {
		t.Fatalf("initiator %#v", sender.got)
	}

	peer := &wallDialog{}
	(&Dao{DialogClient: peer}).stampChatDecor(ctx, 2, mtproto.PEER_USER, 1, &mtproto.Message{
		Out:    false,
		FromId: mtproto.MakePeerUser(1),
		Action: action,
	})
	if peer.got == nil || !peer.got.WallpaperOverridden || peer.got.UserId != 2 {
		t.Fatalf("peer %#v", peer.got)
	}

	oneSided := mtproto.MakeTLMessageActionSetChatWallPaper(&mtproto.MessageAction{
		Wallpaper: paper,
	}).To_MessageAction()
	skipped := &wallDialog{}
	(&Dao{DialogClient: skipped}).stampChatDecor(ctx, 2, mtproto.PEER_USER, 1, &mtproto.Message{
		Out:    false,
		FromId: mtproto.MakePeerUser(1),
		Action: oneSided,
	})
	if skipped.got != nil {
		t.Fatalf("one-sided wallpaper was stored for the peer: %#v", skipped.got)
	}
	own := &wallDialog{}
	(&Dao{DialogClient: own}).stampChatDecor(ctx, 1, mtproto.PEER_USER, 2, &mtproto.Message{
		Out:    true,
		FromId: mtproto.MakePeerUser(1),
		Action: oneSided,
	})
	if own.got == nil || own.got.WallpaperOverridden {
		t.Fatalf("one-sided initiator %#v", own.got)
	}
}

type wallDialog struct {
	dialogclient.DialogClient
	got *dialog.TLDialogSetChatWallpaper
}

func (w *wallDialog) DialogSetChatWallpaper(_ context.Context, in *dialog.TLDialogSetChatWallpaper) (*mtproto.Bool, error) {
	w.got = in
	return mtproto.ToBool(true), nil
}
