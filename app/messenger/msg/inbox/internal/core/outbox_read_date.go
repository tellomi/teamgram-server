package core

import (
	"time"

	"github.com/teamgram/teamgram-server/app/messenger/msg/internal/dal/dataobject"
	"github.com/teamgram/teamgram-server/pkg/readtime"
)

// rememberOutboxRead stores when reader covered owner's outbox up to maxID.
// A failure is logged and ignored so a bad receipt write cannot fail
// messages.readHistory or stall the client's sync.
func (c *InboxCore) rememberOutboxRead(owner, reader, peerDialog int64, maxID int32) {
	when := readtime.Stamp(time.Now().Unix())
	if owner == 0 || reader == 0 || owner == reader || peerDialog == 0 || maxID <= 0 || when <= 0 {
		return
	}
	_, _, err := c.svcCtx.Dao.MessageReadOutboxDAO.InsertOrUpdate(
		c.ctx,
		&dataobject.MessageReadOutboxDO{
			UserId:            owner,
			PeerDialogId:      peerDialog,
			ReadUserId:        reader,
			ReadOutboxMaxId:   maxID,
			ReadOutboxMaxDate: when,
		})
	if err != nil {
		c.Logger.Errorf("inbox.rememberOutboxRead - error: %v", err)
	}
}
