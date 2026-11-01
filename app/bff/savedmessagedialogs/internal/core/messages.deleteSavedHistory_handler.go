package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/savedmessagedialogs/saved"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
)

// MessagesDeleteSavedHistory
// messages.deleteSavedHistory#4dc5085f flags:# parent_peer:flags.0?InputPeer peer:InputPeer max_id:int min_date:flags.2?int max_date:flags.3?int = messages.AffectedHistory;
// https://core.telegram.org/method/messages.deleteSavedHistory
//
// Deletes messages forwarded from peer into Saved Messages, up to max_id and
// inside the optional date window. One call removes at most 100 messages.
// offset is 1 when another call is required.
func (c *SavedMessageDialogsCore) MessagesDeleteSavedHistory(in *mtproto.TLMessagesDeleteSavedHistory) (*mtproto.Messages_AffectedHistory, error) {
	if err := c.requireUser(); err != nil {
		return nil, err
	}
	if saved.ParentSpecified(in.GetParentPeer()) {
		if err := c.requireMonoforum(in.GetParentPeer()); err != nil {
			c.Logger.Errorf("messages.deleteSavedHistory - error: %v", err)
			return nil, err
		}
		return c.affected(c.currentPts(), 0, 0), nil
	}
	peer, err := c.savedPeer(in.GetPeer())
	if err != nil {
		c.Logger.Errorf("messages.deleteSavedHistory - error: %v", err)
		return nil, err
	}
	if in.GetMaxId() < 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	listed, err := c.svcCtx.Dao.MessageGetSavedHistoryMessages(c.ctx, &message.TLMessageGetSavedHistoryMessages{
		UserId:     c.MD.UserId,
		PeerType:   peer.PeerType,
		PeerId:     peer.PeerId,
		MaxId:      in.GetMaxId(),
		OffsetDate: dateValue(in.GetMinDate()),
		AddOffset:  dateValue(in.GetMaxDate()),
		Limit:      message.SavedHistoryDeleteList,
	})
	if err != nil {
		c.Logger.Errorf("messages.deleteSavedHistory - error: %v", err)
		return nil, err
	}
	var ids []int32
	if listed != nil {
		for _, box := range listed.BoxList {
			if box != nil && box.MessageId != 0 {
				ids = append(ids, box.MessageId)
			}
		}
	}
	if len(ids) == 0 {
		return c.affected(c.currentPts(), 0, 0), nil
	}
	deleted, err := c.svcCtx.Dao.MsgDeleteMessages(c.ctx, &msgpb.TLMsgDeleteMessages{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
		PeerType:  mtproto.PEER_EMPTY,
		Revoke:    false,
		Id:        ids,
	})
	if err != nil {
		c.Logger.Errorf("messages.deleteSavedHistory - error: %v", err)
		return nil, err
	}
	if deleted == nil || deleted.PtsCount == 0 {
		return c.affected(c.currentPts(), 0, 0), nil
	}
	top, err := c.newestSaved(peer)
	if err != nil {
		c.Logger.Errorf("messages.deleteSavedHistory - error: %v", err)
		return nil, err
	}
	if err := c.touchSaved(peer, top); err != nil {
		c.Logger.Errorf("messages.deleteSavedHistory - error: %v", err)
		return nil, err
	}
	offset := int32(0)
	if int32(len(ids)) >= saved.DeleteBatch {
		offset = 1
	}
	pts, ptsCount := c.currentPts(), int32(0)
	if deleted != nil {
		pts = deleted.Pts
		ptsCount = deleted.PtsCount
	}
	return c.affected(pts, ptsCount, offset), nil
}

func (c *SavedMessageDialogsCore) newestSaved(peer *mtproto.PeerUtil) (int32, error) {
	list, err := c.svcCtx.Dao.MessageGetSavedHistoryMessages(c.ctx, &message.TLMessageGetSavedHistoryMessages{
		UserId:   c.MD.UserId,
		PeerType: peer.PeerType,
		PeerId:   peer.PeerId,
		Limit:    1,
	})
	if err != nil || list == nil || len(list.BoxList) == 0 || list.BoxList[0] == nil {
		return 0, err
	}
	return list.BoxList[0].MessageId, nil
}

func (c *SavedMessageDialogsCore) currentPts() int32 {
	st, err := c.svcCtx.Dao.UpdatesGetStateV2(c.ctx, &updates.TLUpdatesGetStateV2{
		UserId:    c.MD.UserId,
		AuthKeyId: c.MD.PermAuthKeyId,
	})
	if err != nil || st == nil {
		return 0
	}
	return st.Pts
}

func (c *SavedMessageDialogsCore) affected(pts, ptsCount, offset int32) *mtproto.Messages_AffectedHistory {
	return mtproto.MakeTLMessagesAffectedHistory(&mtproto.Messages_AffectedHistory{
		Pts:      pts,
		PtsCount: ptsCount,
		Offset:   offset,
	}).To_Messages_AffectedHistory()
}
