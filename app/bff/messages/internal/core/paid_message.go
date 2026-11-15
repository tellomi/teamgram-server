package core

import (
	"fmt"

	"github.com/teamgram/proto/mtproto"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc/status"
)

// paidMessageRequired enforces globalPrivacySettings.noncontact_peers_paid_stars.
// A contact of the recipient sends for free. Anyone else must set
// allow_paid_stars to at least that price. This server does not charge Stars;
// the official error tells the client the message is paid.
func (c *MessagesCore) paidMessageRequired(peer *mtproto.PeerUtil, allowed int64) error {
	if c == nil || c.MD == nil || peer == nil || !peer.IsUser() || peer.PeerId == 0 || peer.PeerId == c.MD.UserId {
		return nil
	}
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil
	}
	settings, err := c.svcCtx.Dao.UserClient.UserGetGlobalPrivacySettings(c.ctx, &userpb.TLUserGetGlobalPrivacySettings{UserId: peer.PeerId})
	if err != nil || settings == nil {
		return nil
	}
	price := settings.GetNoncontactPeersPaidStars().GetValue()
	if price <= 0 {
		return nil
	}
	contact, err := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &userpb.TLUserCheckContact{
		UserId: peer.PeerId,
		Id:     c.MD.UserId,
	})
	if err == nil && mtproto.FromBool(contact) {
		return nil
	}
	if allowed >= price {
		return nil
	}
	return status.Error(mtproto.ErrForbidden, fmt.Sprintf("ALLOW_PAYMENT_REQUIRED_%d", price))
}
