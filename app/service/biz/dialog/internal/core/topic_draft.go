package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/internal/dal/dataobject"

	"github.com/zeromicro/go-zero/core/jsonx"
)

func (c *DialogCore) topicDraftsReady() bool {
	return c != nil && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.TopicDraftsDAO != nil
}

// saveTopicDraft writes one forum topic. top_msg_id 0 is the dialog row and
// never reaches this function. An empty draft clears only this topic.
func (c *DialogCore) saveTopicDraft(in *dialog.TLDialogSaveDraftMessage, top int32) (*mtproto.Bool, error) {
	if !c.topicDraftsReady() {
		return nil, mtproto.ErrInternalServerError
	}
	msg := in.GetMessage()
	if msg != nil {
		msg.SanitizeEntities()
	}
	dialog.EnsureTopicReply(msg, top)
	raw, err := jsonx.Marshal(msg)
	if err != nil {
		c.Logger.Errorf("dialog.saveDraftMessage - topic %d: %v", top, err)
		return nil, err
	}
	now := time.Now().Unix()
	if dialog.DraftIsEmpty(msg) {
		n, err := c.svcCtx.Dao.TopicDraftsDAO.ClearOne(c.ctx, in.UserId, in.PeerType, in.PeerId, top, string(raw), now)
		if err != nil {
			c.Logger.Errorf("dialog.saveDraftMessage - clear topic %d: %v", top, err)
			return nil, err
		}
		if n > 0 {
			return mtproto.BoolTrue, nil
		}
		return mtproto.BoolFalse, nil
	}
	err = c.svcCtx.Dao.TopicDraftsDAO.Upsert(c.ctx, &dataobject.DialogTopicDraftDO{
		UserId:           in.UserId,
		PeerType:         in.PeerType,
		PeerId:           in.PeerId,
		TopMsgId:         top,
		DraftType:        2,
		DraftMessageData: string(raw),
		Date2:            now,
	})
	if err != nil {
		c.Logger.Errorf("dialog.saveDraftMessage - topic %d: %v", top, err)
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func peerWithTopicDraft(row *dataobject.DialogTopicDraftDO) (*dialog.PeerWithDraftMessage, error) {
	if row == nil || row.TopMsgId <= 0 || row.DraftType <= 0 {
		return nil, nil
	}
	draft, err := decodeStoredDraft(row.DraftMessageData)
	if err != nil {
		return nil, err
	}
	if draft == nil {
		draft = mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{}).To_DraftMessage()
	}
	dialog.EnsureTopicReply(draft, row.TopMsgId)
	return dialog.MakeTLUpdateDraftMessage(&dialog.PeerWithDraftMessage{
		Peer:  mtproto.MakePeer(row.PeerType, row.PeerId),
		Draft: draft,
	}).To_PeerWithDraftMessage(), nil
}

func (c *DialogCore) loadTopicDrafts(userId int64) ([]*dialog.PeerWithDraftMessage, error) {
	if !c.topicDraftsReady() {
		return nil, nil
	}
	rows, err := c.svcCtx.Dao.TopicDraftsDAO.SelectByUser(c.ctx, userId)
	if err != nil {
		return nil, err
	}
	out := make([]*dialog.PeerWithDraftMessage, 0, len(rows))
	for i := range rows {
		item, err := peerWithTopicDraft(&rows[i])
		if err != nil {
			c.Logger.Errorf("dialog topic draft (%d, %d, %d): %v", rows[i].PeerType, rows[i].PeerId, rows[i].TopMsgId, err)
			continue
		}
		if item != nil {
			out = append(out, item)
		}
	}
	return out, nil
}

func emptyTopicDraft(top int32) *mtproto.DraftMessage {
	draft := mtproto.MakeTLDraftMessageEmpty(&mtproto.DraftMessage{
		Date_FLAGINT32: mtproto.MakeFlagsInt32(int32(time.Now().Unix())),
	}).To_DraftMessage()
	dialog.EnsureTopicReply(draft, top)
	return draft
}
