package core

import (
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// globalFetchDate is the SQL ceiling for both message sources.
// Zero means the first page. The value is offset_rate (a unix date), never
// offset_id. A channel message id and a user box id are different spaces.
func globalFetchDate(cur globalCursor) int32 {
	if !cur.Active || cur.Rate <= 0 {
		return 0
	}
	return cur.Rate
}

// globalCursor is messages.searchGlobal's (offset_rate, offset_peer, offset_id).
// Results are ordered by date descending, then peer type, peer id, then message id descending.
type globalCursor struct {
	Active   bool
	Rate     int32
	PeerType int32
	PeerID   int64
	MsgID    int32
}

func olderThanCursor(date int32, peerType int32, peerID int64, msgID int32, cur globalCursor) bool {
	if !cur.Active {
		return true
	}
	if date != cur.Rate {
		return date < cur.Rate
	}
	if peerType != cur.PeerType {
		return peerType > cur.PeerType
	}
	if peerID != cur.PeerID {
		return peerID > cur.PeerID
	}
	return msgID < cur.MsgID
}

func peerKey(peerType int32, peerID int64) string {
	return strconv.FormatInt(int64(peerType), 10) + ":" + strconv.FormatInt(peerID, 10)
}

func filterGlobalPage(list *mtproto.MessageBoxList, cur globalCursor, folder map[string]bool, useFolder bool, limit int32) *mtproto.MessageBoxList {
	if list == nil {
		return list
	}
	kept := make([]*mtproto.MessageBox, 0, len(list.BoxList))
	for _, box := range list.BoxList {
		if box == nil {
			continue
		}
		if useFolder && !folder[peerKey(box.GetPeerType(), box.GetPeerId())] {
			continue
		}
		date := int32(0)
		msgID := box.GetMessageId()
		if box.GetMessage() != nil {
			date = box.GetMessage().GetDate()
			if box.GetMessage().GetId() != 0 {
				msgID = box.GetMessage().GetId()
			}
		}
		if !olderThanCursor(date, box.GetPeerType(), box.GetPeerId(), msgID, cur) {
			continue
		}
		kept = append(kept, box)
		if limit > 0 && int32(len(kept)) >= limit {
			break
		}
	}
	list.BoxList = kept
	return list
}

// attachChannelFolders adds joined channels to the folder the client asked
// for. Channel dialogs are not rows in the user dialog table, so a folder_id
// (Android always sends one) used to drop every channel post. Folder 0 is
// the main list. A channel that is archived (folder 1) stays out of it.
func (c *MessagesCore) attachChannelFolders(folder map[string]bool, folderID int32) map[string]bool {
	if c == nil || c.MD == nil {
		return folder
	}
	if folder == nil {
		folder = map[string]bool{}
	}
	rows, err := channel.Shared().DialogsFor(c.ctx, c.MD.UserId)
	if err != nil {
		c.Logger.Errorf("messages.searchGlobal channel folders: %v", err)
		return folder
	}
	archived := map[int64]bool{}
	if c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.DialogClient != nil {
		got, err := c.svcCtx.Dao.DialogGetDialogs(c.ctx, &dialog.TLDialogGetDialogs{
			UserId:        c.MD.UserId,
			ExcludePinned: mtproto.BoolFalse,
			FolderId:      1,
		})
		if err != nil {
			c.Logger.Errorf("messages.searchGlobal archive: %v", err)
		} else {
			for _, row := range got.GetDatas() {
				if row.GetDialog().GetPeer() == nil {
					continue
				}
				peer := mtproto.FromPeer(row.GetDialog().GetPeer())
				if peer != nil && peer.PeerType == mtproto.PEER_CHANNEL && peer.PeerId != 0 {
					archived[peer.PeerId] = true
				}
			}
		}
	}
	ids := make([]int64, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ChannelID)
	}
	addJoinedChannels(folder, folderID, ids, archived)
	return folder
}

// addJoinedChannels puts joined channel ids into folder for this folder_id.
// archived lists channel ids that live in folder 1.
func addJoinedChannels(folder map[string]bool, folderID int32, joined []int64, archived map[int64]bool) {
	if folder == nil {
		return
	}
	for _, id := range joined {
		if id == 0 {
			continue
		}
		inArchive := archived[id]
		if folderID == 0 && inArchive {
			continue
		}
		if folderID != 0 && !inArchive {
			continue
		}
		folder[peerKey(mtproto.PEER_CHANNEL, id)] = true
	}
}

func (c *MessagesCore) globalFolder(folderID int32) (map[string]bool, bool) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return nil, false
	}
	rows, err := c.svcCtx.Dao.DialogGetDialogs(c.ctx, &dialog.TLDialogGetDialogs{
		UserId:        c.MD.UserId,
		ExcludePinned: mtproto.BoolFalse,
		FolderId:      folderID,
	})
	if err != nil || rows == nil {
		c.Logger.Errorf("messages.searchGlobal folder %d: %v", folderID, err)
		return nil, false
	}
	out := map[string]bool{}
	for _, row := range rows.GetDatas() {
		if row.GetDialog().GetPeer() == nil {
			continue
		}
		peer := mtproto.FromPeer(row.GetDialog().GetPeer())
		if peer == nil || peer.PeerId == 0 {
			continue
		}
		out[peerKey(peer.PeerType, peer.PeerId)] = true
	}
	return out, true
}
