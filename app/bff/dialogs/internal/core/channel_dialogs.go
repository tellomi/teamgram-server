package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

func (c *DialogsCore) mergeChannelDialogs(list dialog.DialogExtList, folderID int32, excludePinned bool) dialog.DialogExtList {
	if c.svcCtx.Plugin == nil {
		return keepFolder(list, folderID)
	}
	extras, err := c.svcCtx.Plugin.ListChannelDialogs(c.ctx, c.MD.UserId)
	if err != nil || len(extras) == 0 {
		return keepFolder(list, folderID)
	}
	c.stampChannelFolders(extras)
	if excludePinned {
		extras = withoutPinnedDialogs(extras)
	}
	have := map[int64]bool{}
	for _, ext := range extras {
		if peer := ext.GetDialog().GetPeer(); peer != nil {
			have[peer.GetChannelId()] = true
		}
	}
	out := make(dialog.DialogExtList, 0, len(list)+len(extras))
	for _, ext := range list {
		peer := ext.GetDialog().GetPeer()
		if peer != nil && mtproto.PeerIsChannel(peer) && have[peer.GetChannelId()] {
			continue
		}
		out = append(out, ext)
	}
	for _, ext := range extras {
		if ext == nil || ext.GetDialog() == nil {
			continue
		}
		if ext.GetDialog().GetFolderId().GetValue() != folderID {
			continue
		}
		out = append(out, ext)
	}
	return keepFolder(out, folderID)
}

// stampChannelFolders copies dialogs.folder_id onto channel dialogs. The
// channel store does not keep the archive bit, so a bare channel dialog would
// always be reported in the main list.
func (c *DialogsCore) stampChannelFolders(list dialog.DialogExtList) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || len(list) == 0 {
		return
	}
	ids := make([]int64, 0, len(list))
	for _, ext := range list {
		peer := ext.GetDialog().GetPeer()
		if peer == nil || peer.GetChannelId() == 0 {
			continue
		}
		ids = append(ids, mtproto.MakePeerDialogId(mtproto.PEER_CHANNEL, peer.GetChannelId()))
	}
	if len(ids) == 0 {
		return
	}
	rows, err := c.svcCtx.Dao.DialogClient.DialogGetDialogsByIdList(c.ctx, &dialog.TLDialogGetDialogsByIdList{
		UserId: c.MD.UserId,
		IdList: ids,
	})
	if err != nil || rows == nil {
		return
	}
	applyChannelDialogRows(list, rows.GetDatas())
}

// applyChannelDialogRows copies the dialogs-table folder and pin onto channel
// dialogs. The channel store does not keep either bit. A pinned channel was
// replaced by an unpinned row, so getDialogs neither set pinned nor sorted it
// first. Pin rank stays on Order; the chat-list sort reads that before date.
func applyChannelDialogRows(list, rows dialog.DialogExtList) {
	type saved struct {
		folder int32
		pinned bool
		order  int64
	}
	state := map[int64]saved{}
	for _, row := range rows {
		peer := row.GetDialog().GetPeer()
		if peer == nil || peer.GetChannelId() == 0 {
			continue
		}
		state[peer.GetChannelId()] = saved{
			folder: row.GetDialog().GetFolderId().GetValue(),
			pinned: row.GetDialog().GetPinned(),
			order:  row.GetOrder(),
		}
	}
	for _, ext := range list {
		peer := ext.GetDialog().GetPeer()
		if peer == nil || ext.GetDialog() == nil {
			continue
		}
		row, ok := state[peer.GetChannelId()]
		if !ok {
			continue
		}
		ext.Dialog.FolderId = mtproto.MakeFlagsInt32(row.folder)
		if row.pinned {
			ext.Dialog.Pinned = true
			ext.Order = row.order
		}
	}
}

func withoutPinnedDialogs(list dialog.DialogExtList) dialog.DialogExtList {
	kept := list[:0]
	for _, ext := range list {
		if ext == nil || ext.GetDialog().GetPinned() {
			continue
		}
		kept = append(kept, ext)
	}
	return kept
}

// applyChannelLiveRows copies the channel store onto dialogs-table rows.
// The table top only moves forward, so after the newest post is deleted it
// still names that id and getDialogs / getPinnedDialogs drop the chat. The
// channel store has the live top. Pin rank and folder stay on the table row.
func applyChannelLiveRows(stored, live dialog.DialogExtList) {
	byID := map[int64]*dialog.DialogExt{}
	for _, ext := range live {
		peer := ext.GetDialog().GetPeer()
		if peer == nil || peer.GetChannelId() == 0 {
			continue
		}
		byID[peer.GetChannelId()] = ext
	}
	for _, ext := range stored {
		d := ext.GetDialog()
		if d == nil || d.GetPeer() == nil || d.GetPeer().GetChannelId() == 0 {
			continue
		}
		src := byID[d.GetPeer().GetChannelId()]
		if src == nil || src.GetDialog() == nil {
			continue
		}
		s := src.GetDialog()
		d.TopMessage = s.GetTopMessage()
		d.ReadInboxMaxId = s.GetReadInboxMaxId()
		d.ReadOutboxMaxId = s.GetReadOutboxMaxId()
		d.UnreadCount = s.GetUnreadCount()
		d.Pts = s.Pts
		d.ViewForumAsMessages = s.GetViewForumAsMessages()
		ext.Date = src.GetDate()
		if !d.GetPinned() {
			ext.Order = src.GetOrder()
		}
	}
}

func (c *DialogsCore) applyChannelLive(list dialog.DialogExtList) dialog.DialogExtList {
	if c == nil || c.svcCtx == nil || c.svcCtx.Plugin == nil || len(list) == 0 {
		return list
	}
	live, err := c.svcCtx.Plugin.ListChannelDialogs(c.ctx, c.MD.UserId)
	if err != nil || len(live) == 0 {
		return list
	}
	applyChannelLiveRows(list, live)
	return list
}

func keepFolder(list dialog.DialogExtList, folderID int32) dialog.DialogExtList {
	out := make(dialog.DialogExtList, 0, len(list))
	for _, ext := range list {
		if ext == nil || ext.GetDialog() == nil {
			continue
		}
		if ext.GetDialog().GetPredicateName() == mtproto.Predicate_dialogFolder {
			if folderID == 0 {
				out = append(out, ext)
			}
			continue
		}
		if ext.GetDialog().GetFolderId().GetValue() == folderID {
			out = append(out, ext)
		}
	}
	return out
}

func (c *DialogsCore) ensureChannelPeers(list dialog.DialogExtList, peers []*mtproto.PeerUtil) dialog.DialogExtList {
	if c.svcCtx.Plugin == nil {
		return list
	}
	have := map[int64]bool{}
	for _, ext := range list {
		if peer := ext.GetDialog().GetPeer(); peer != nil && mtproto.PeerIsChannel(peer) {
			have[peer.GetChannelId()] = true
		}
	}
	for _, peer := range peers {
		if peer == nil || !peer.IsChannel() || have[peer.PeerId] {
			continue
		}
		ext, err := c.svcCtx.Plugin.GetChannelDialogById(c.ctx, c.MD.UserId, peer.PeerId)
		if err == nil && ext != nil {
			c.stampChannelFolders(dialog.DialogExtList{ext})
			list = append(list, ext)
			have[peer.PeerId] = true
		}
	}
	return list
}

func (c *DialogsCore) channelMessages(ctx context.Context, self int64, ids []dialog.TopMessageId) []*mtproto.Message {
	if c.svcCtx.Plugin == nil {
		return nil
	}
	var out []*mtproto.Message
	for _, id := range ids {
		if !id.Peer.IsChannel() || id.TopMessage == 0 {
			continue
		}
		box, err := c.svcCtx.Plugin.GetChannelMessage(ctx, self, id.Peer.PeerId, id.TopMessage)
		if err == nil && box != nil {
			out = append(out, box.ToMessage(self))
		}
	}
	return out
}

func (c *DialogsCore) channelChats(ctx context.Context, self int64, ids ...int64) []*mtproto.Chat {
	if c.svcCtx.Plugin == nil || len(ids) == 0 {
		return []*mtproto.Chat{}
	}
	return c.svcCtx.Plugin.GetChannelListByIdList(ctx, self, ids...)
}
