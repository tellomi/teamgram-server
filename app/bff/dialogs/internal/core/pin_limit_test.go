package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/premium"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestTogglePinRejectsTheFreeCap(t *testing.T) {
	freePinEnv(t)
	box := &pinBox{rows: pinRows(premium.PinnedDialogsFree)}
	c := newPinCore(box, false)
	_, err := c.MessagesToggleDialogPin(pinToggle(9000, true))
	if err != mtproto.ErrPinnedDialogsTooMuch {
		t.Fatalf("sixth free pin: %v", err)
	}
	if box.toggles != 0 {
		t.Fatalf("toggle ran %d times", box.toggles)
	}
}

func TestTogglePinAllowsAMemberPastTheFreeCap(t *testing.T) {
	freePinEnv(t)
	box := &pinBox{rows: pinRows(premium.PinnedDialogsFree)}
	c := newPinCore(box, true)
	if _, err := c.MessagesToggleDialogPin(pinToggle(9000, true)); err != nil {
		t.Fatal(err)
	}
	if box.toggles != 1 {
		t.Fatalf("toggles %d", box.toggles)
	}
}

func TestToggleUnpinAndRepeatDoNotTakeASlot(t *testing.T) {
	freePinEnv(t)
	box := &pinBox{rows: pinRows(premium.PinnedDialogsFree), already: true}
	c := newPinCore(box, false)
	if _, err := c.MessagesToggleDialogPin(pinToggle(9000, true)); err != nil || box.toggles != 1 {
		t.Fatalf("repeat pin err %v toggles %d", err, box.toggles)
	}
	if _, err := c.MessagesToggleDialogPin(pinToggle(9000, false)); err != nil || box.toggles != 2 {
		t.Fatalf("unpin err %v toggles %d", err, box.toggles)
	}
}

func TestTogglePinRejectsTheArchiveCap(t *testing.T) {
	freePinEnv(t)
	box := &pinBox{folder: 1, rows: pinRows(premium.FolderPinnedFree)}
	c := newPinCore(box, false)
	if _, err := c.MessagesToggleDialogPin(pinToggle(9000, true)); err != mtproto.ErrPinnedDialogsTooMuch {
		t.Fatalf("archive: %v", err)
	}
	box.rows = pinRows(premium.FolderPinnedFree - 1)
	if _, err := c.MessagesToggleDialogPin(pinToggle(9000, true)); err != nil || box.toggles != 1 {
		t.Fatalf("under archive cap err %v toggles %d", err, box.toggles)
	}
}

func TestReorderRejectsAnOrderPastTheCap(t *testing.T) {
	freePinEnv(t)
	box := &pinBox{}
	c := newPinCore(box, false)
	_, err := c.MessagesReorderPinnedDialogs(&mtproto.TLMessagesReorderPinnedDialogs{
		Force: true,
		Order: pinOrder(premium.PinnedDialogsFree + 1),
	})
	if err != mtproto.ErrPinnedDialogsTooMuch || box.reorders != 0 {
		t.Fatalf("err %v reorders %d", err, box.reorders)
	}
	if _, err = c.MessagesReorderPinnedDialogs(&mtproto.TLMessagesReorderPinnedDialogs{
		Force: true,
		Order: pinOrder(premium.PinnedDialogsFree),
	}); err != nil || box.reorders != 1 {
		t.Fatalf("exact cap err %v reorders %d", err, box.reorders)
	}
}

func TestReorderWithoutForceKeepsPinsTheClientLeftOut(t *testing.T) {
	freePinEnv(t)
	box := &pinBox{rows: pinRows(premium.PinnedDialogsFree)}
	c := newPinCore(box, false)
	_, err := c.MessagesReorderPinnedDialogs(&mtproto.TLMessagesReorderPinnedDialogs{
		Order: pinOrder(1),
	})
	if err != mtproto.ErrPinnedDialogsTooMuch || box.reorders != 0 {
		t.Fatalf("kept pins err %v reorders %d", err, box.reorders)
	}
}

func TestReorderRejectsTheArchiveCap(t *testing.T) {
	freePinEnv(t)
	box := &pinBox{}
	c := newPinCore(box, false)
	_, err := c.MessagesReorderPinnedDialogs(&mtproto.TLMessagesReorderPinnedDialogs{
		Force:    true,
		FolderId: 1,
		Order:    pinOrder(premium.FolderPinnedFree + 1),
	})
	if err != mtproto.ErrPinnedDialogsTooMuch || box.reorders != 0 {
		t.Fatalf("archive reorder %v reorders %d", err, box.reorders)
	}
	_, err = c.MessagesReorderPinnedDialogs(&mtproto.TLMessagesReorderPinnedDialogs{
		Force:    true,
		FolderId: 1,
		Order:    pinOrder(premium.FolderPinnedDialogs),
	})
	if err != mtproto.ErrPinnedDialogsTooMuch || box.reorders != 0 {
		t.Fatalf("free account accepted the member archive cap: %v reorders %d", err, box.reorders)
	}
	if _, err = c.MessagesReorderPinnedDialogs(&mtproto.TLMessagesReorderPinnedDialogs{
		Force:    true,
		FolderId: 1,
		Order:    pinOrder(premium.FolderPinnedFree),
	}); err != nil || box.reorders != 1 {
		t.Fatalf("exact archive cap err %v reorders %d", err, box.reorders)
	}
}

func TestPinLimitFollowsTheAdvertisedAppConfig(t *testing.T) {
	freePinEnv(t)
	def, prem := premium.Advertised(premium.FeaturePinnedChats)
	if def != premium.PinnedDialogsFree || prem != premium.PinnedDialogs {
		t.Fatalf("dialogs_pinned_limit %d/%d", def, prem)
	}
	fdef, fprem := premium.Advertised(premium.FeatureFolderPins)
	if fdef != premium.FolderPinnedFree || fprem != premium.FolderPinnedDialogs {
		t.Fatalf("dialogs_folder_pinned_limit %d/%d", fdef, fprem)
	}
	if premium.Limit(premium.FeaturePinnedChats, false) != def || premium.Limit(premium.FeaturePinnedChats, true) != prem {
		t.Fatal("enforced cap drifted from the advertised pair")
	}
	if !pinSlotTaken(false, def, def) || pinSlotTaken(true, def, def) || reorderPastLimit(true, nil, pinIDs(def), def) {
		t.Fatal("boundary")
	}
	if !reorderPastLimit(false, pinIDs(def), []int64{99}, def) || reorderPastLimit(false, pinIDs(def), pinIDs(1), def) {
		t.Fatal("unlisted pins were dropped or a repeat was counted twice")
	}
}

func freePinEnv(t *testing.T) {
	t.Helper()
	t.Cleanup(func() { premium.Reload() })
	t.Setenv(premium.EnvEveryone, "false")
	premium.Reload()
}

func newPinCore(box *pinBox, member bool) *DialogsCore {
	ctx := context.Background()
	users := &pinUsers{premium: member}
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: box,
			UserClient:   users,
			SyncClient:   pinSync{},
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 9},
	}
}

func pinToggle(userID int64, pinned bool) *mtproto.TLMessagesToggleDialogPin {
	return &mtproto.TLMessagesToggleDialogPin{
		Pinned: pinned,
		Peer: mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
			Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: userID}).To_InputPeer(),
		}).To_InputDialogPeer(),
	}
}

func pinOrder(n int) []*mtproto.InputDialogPeer {
	order := make([]*mtproto.InputDialogPeer, n)
	for i := range order {
		order[i] = mtproto.MakeTLInputDialogPeer(&mtproto.InputDialogPeer{
			Peer: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: int64(1000 + i)}).To_InputPeer(),
		}).To_InputDialogPeer()
	}
	return order
}

func pinIDs(n int) []int64 {
	ids := make([]int64, n)
	for i := range ids {
		ids[i] = int64(i + 1)
	}
	return ids
}

func pinRows(n int) []*dialog.DialogExt {
	rows := make([]*dialog.DialogExt, n)
	for i := range rows {
		rows[i] = dialog.MakeTLDialogExt(&dialog.DialogExt{
			Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
				Pinned: true,
				Peer:   mtproto.MakePeerUser(int64(i + 1)),
			}).To_Dialog(),
		}).To_DialogExt()
	}
	return rows
}

type pinBox struct {
	dialogclient.DialogClient
	rows     []*dialog.DialogExt
	folder   int32
	already  bool
	toggles  int
	reorders int
}

func (b *pinBox) DialogGetDialogById(_ context.Context, in *dialog.TLDialogGetDialogById) (*dialog.DialogExt, error) {
	return dialog.MakeTLDialogExt(&dialog.DialogExt{
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Pinned:   b.already,
			Peer:     mtproto.MakePeerUser(in.GetPeerId()),
			FolderId: mtproto.MakeFlagsInt32(b.folder),
		}).To_Dialog(),
	}).To_DialogExt(), nil
}

func (b *pinBox) DialogGetPinnedDialogs(context.Context, *dialog.TLDialogGetPinnedDialogs) (*dialog.Vector_DialogExt, error) {
	return &dialog.Vector_DialogExt{Datas: b.rows}, nil
}

func (b *pinBox) DialogToggleDialogPin(context.Context, *dialog.TLDialogToggleDialogPin) (*mtproto.Int32, error) {
	b.toggles++
	return &mtproto.Int32{V: b.folder}, nil
}

func (b *pinBox) DialogReorderPinnedDialogs(context.Context, *dialog.TLDialogReorderPinnedDialogs) (*mtproto.Bool, error) {
	b.reorders++
	return mtproto.BoolTrue, nil
}

type pinUsers struct {
	userclient.UserClient
	premium bool
}

func (u *pinUsers) UserGetImmutableUser(context.Context, *user.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{Id: 7, Premium: u.premium, FirstName: "A"}}, nil
}

func (pinUsers) UserGetMutableUsers(context.Context, *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error) {
	return &user.Vector_ImmutableUser{}, nil
}

type pinSync struct{ syncclient.SyncClient }

func (pinSync) SyncUpdatesNotMe(context.Context, *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	return mtproto.EmptyVoid, nil
}
