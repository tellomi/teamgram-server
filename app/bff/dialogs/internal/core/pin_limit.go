package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

// rejectPinPastLimit is messages.toggleDialogPin past the cap help.getAppConfig
// publishes as dialogs_pinned_limit_default / dialogs_pinned_limit_premium.
// Folder 1 uses dialogs_folder_pinned_limit_default / _premium. Unpinning and
// a dialog that is already pinned do not take a new slot.
func (c *DialogsCore) rejectPinPastLimit(peer *mtproto.PeerUtil, pinning bool) error {
	if !pinning || peer == nil || !c.pinLimitReady() {
		return nil
	}
	folder, already := c.pinFolder(peer)
	if already {
		return nil
	}
	pins, err := c.svcCtx.Dao.DialogClient.DialogGetPinnedDialogs(c.ctx, &dialog.TLDialogGetPinnedDialogs{
		UserId:   c.MD.UserId,
		FolderId: folder,
	})
	if err != nil {
		return err
	}
	ids := pinnedDialogIDs(pins.GetDatas())
	want := mtproto.MakePeerDialogId(peer.PeerType, peer.PeerId)
	for _, id := range ids {
		if id == want {
			return nil
		}
	}
	if pinSlotTaken(false, len(ids), c.pinnedDialogLimit(folder)) {
		return mtproto.ErrPinnedDialogsTooMuch
	}
	return nil
}

// rejectReorderPastLimit is messages.reorderPinnedDialogs past the same caps.
// force replaces the pin set with order. Without force, pins the client left
// out stay pinned, so the resulting count is the order plus those pins.
func (c *DialogsCore) rejectReorderPastLimit(folder int32, force bool, order []int64) error {
	if !c.pinLimitReady() {
		return nil
	}
	pins, err := c.svcCtx.Dao.DialogClient.DialogGetPinnedDialogs(c.ctx, &dialog.TLDialogGetPinnedDialogs{
		UserId:   c.MD.UserId,
		FolderId: folder,
	})
	if err != nil {
		return err
	}
	if reorderPastLimit(force, pinnedDialogIDs(pins.GetDatas()), order, c.pinnedDialogLimit(folder)) {
		return mtproto.ErrPinnedDialogsTooMuch
	}
	return nil
}

func (c *DialogsCore) pinLimitReady() bool {
	return c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.DialogClient != nil && c.MD != nil
}

func (c *DialogsCore) pinFolder(peer *mtproto.PeerUtil) (folder int32, already bool) {
	ext, err := c.svcCtx.Dao.DialogClient.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
	})
	if err != nil || ext == nil || ext.GetDialog() == nil {
		return 0, false
	}
	d := ext.GetDialog()
	return d.GetFolderId().GetValue(), d.GetPinned()
}

// pinnedDialogLimit is the cap for this account and folder. The numbers are
// premium.Limit, which help.getAppConfig publishes as dialogs_pinned_limit_*
// and dialogs_folder_pinned_limit_*. Folder 1 is the archive. A user lookup
// failure uses the non-member cap; TELLOMI_PREMIUM_EVERYONE still raises it.
func (c *DialogsCore) pinnedDialogLimit(folder int32) int {
	feature := premium.FeaturePinnedChats
	if folder == 1 {
		feature = premium.FeatureFolderPins
	}
	member := false
	if c.svcCtx.Dao.UserClient != nil && c.MD != nil {
		u, err := c.svcCtx.Dao.UserClient.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: c.MD.UserId})
		if err == nil && u != nil {
			member = u.Premium()
		}
	}
	return premium.Limit(feature, member)
}

func pinnedDialogIDs(rows []*dialog.DialogExt) []int64 {
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		if row == nil || row.GetDialog() == nil || row.GetDialog().GetPeer() == nil {
			continue
		}
		p := mtproto.FromPeer(row.GetDialog().GetPeer())
		if p == nil {
			continue
		}
		ids = append(ids, mtproto.MakePeerDialogId(p.PeerType, p.PeerId))
	}
	return ids
}

// pinSlotTaken reports a new pin that would pass the account cap.
// alreadyPinned is a repeat of a pin the chat already has.
func pinSlotTaken(alreadyPinned bool, current, limit int) bool {
	if alreadyPinned || limit < 0 {
		return false
	}
	return current >= limit
}

// reorderPastLimit matches the dialog service: force keeps only order, and
// without force the pins missing from order stay. Exactly `limit` is allowed.
func reorderPastLimit(force bool, current, order []int64, limit int) bool {
	if limit < 0 {
		return false
	}
	if force {
		return len(order) > limit
	}
	have := make(map[int64]struct{}, len(order))
	for _, id := range order {
		have[id] = struct{}{}
	}
	n := len(order)
	for _, id := range current {
		if _, ok := have[id]; !ok {
			n++
		}
	}
	return n > limit
}
