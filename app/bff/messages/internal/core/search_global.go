package core

import "github.com/teamgram/proto/mtproto"

// globalQuery is the part of messages.searchGlobal that can be applied
// after the text search: peer kind and date. folder_id, offset_rate, and
// offset_peer are still not applied.
type globalQuery struct {
	UsersOnly      bool
	GroupsOnly     bool
	BroadcastsOnly bool
	MinDate        int32
	MaxDate        int32
}

type channelKind struct {
	Broadcast bool
	Megagroup bool
}

func (q globalQuery) narrows() bool {
	return q.UsersOnly || q.GroupsOnly || q.BroadcastsOnly || q.MinDate != 0 || q.MaxDate != 0
}

func (q globalQuery) allows(peerType int32, peerID int64, date int32, kinds map[int64]channelKind) bool {
	if q.MinDate != 0 && date < q.MinDate {
		return false
	}
	if q.MaxDate != 0 && date > q.MaxDate {
		return false
	}
	if !q.UsersOnly && !q.GroupsOnly && !q.BroadcastsOnly {
		return true
	}
	switch peerType {
	case mtproto.PEER_USER, mtproto.PEER_SELF:
		return q.UsersOnly
	case mtproto.PEER_CHAT:
		return q.GroupsOnly
	case mtproto.PEER_CHANNEL:
		kind, ok := kinds[peerID]
		if !ok {
			return false
		}
		if kind.Broadcast && q.BroadcastsOnly {
			return true
		}
		if kind.Megagroup && q.GroupsOnly {
			return true
		}
		return false
	default:
		return false
	}
}

func filterGlobalBoxes(list *mtproto.MessageBoxList, q globalQuery, kinds map[int64]channelKind, limit int32) *mtproto.MessageBoxList {
	if list == nil {
		return list
	}
	kept := make([]*mtproto.MessageBox, 0, len(list.BoxList))
	for _, box := range list.BoxList {
		if box == nil {
			continue
		}
		date := int32(0)
		if box.GetMessage() != nil {
			date = box.GetMessage().GetDate()
		}
		if !q.allows(box.GetPeerType(), box.GetPeerId(), date, kinds) {
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
