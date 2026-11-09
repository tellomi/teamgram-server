package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/enforcement"
)

func (c *MessagesCore) blockSend(peer *mtproto.PeerUtil) error {
	if c == nil || c.MD == nil || peer == nil {
		return nil
	}
	stranger := peer.IsUser() && !peer.IsSelfUser(c.MD.UserId)
	if stranger && enforcement.ContactMatters(c.MD.UserId, time.Now()) && c.svcCtx != nil && c.svcCtx.Dao != nil && c.svcCtx.Dao.UserClient != nil {
		contact, err := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &userpb.TLUserCheckContact{
			UserId: c.MD.UserId,
			Id:     peer.PeerId,
		})
		if err == nil && mtproto.FromBool(contact) {
			stranger = false
		}
	}
	return enforcement.Reject(c.MD.UserId, "send_message", stranger, time.Now())
}
