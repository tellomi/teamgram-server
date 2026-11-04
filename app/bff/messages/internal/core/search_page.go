package core

import (
	"strconv"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

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
