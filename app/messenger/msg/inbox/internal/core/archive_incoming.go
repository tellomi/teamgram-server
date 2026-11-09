package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/msg/inbox/inbox"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/archive"

	"google.golang.org/grpc/status"
)

func inboxDialogPeer(in *inbox.TLInboxSendUserMessageToInboxV2) (int32, int64) {
	if in.GetPeerType() == mtproto.PEER_USER {
		return mtproto.PEER_USER, in.GetFromId()
	}
	return in.GetPeerType(), in.GetPeerId()
}

func inboxPeerClass(peerType int32) archive.PeerClass {
	if peerType == mtproto.PEER_CHAT {
		return archive.ClassGroup
	}
	return archive.ClassUser
}

func (c *InboxCore) userClass(owner, peer int64) archive.PeerClass {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || peer == 0 {
		return archive.ClassUser
	}
	if bot, err := c.svcCtx.Dao.UserClient.UserIsBot(c.ctx, &user.TLUserIsBot{Id: peer}); err == nil && mtproto.FromBool(bot) {
		return archive.ClassBot
	}
	if contact, err := c.svcCtx.Dao.UserClient.UserCheckContact(c.ctx, &user.TLUserCheckContact{UserId: owner, Id: peer}); err == nil && mtproto.FromBool(contact) {
		return archive.ClassContact
	}
	return archive.ClassUser
}

// inboxFolder reports whether the recipient already has this dialog and which
// folder it is in. A missing row is a new dialog. Any other error is treated
// as an existing main-list dialog so a blip cannot archive a live chat.
func (c *InboxCore) inboxFolder(userID int64, peerType int32, peerID int64) (existed bool, folderID int32) {
	if c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil || userID == 0 || peerID == 0 {
		return true, 0
	}
	ext, err := c.svcCtx.Dao.DialogClient.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
		UserId:   userID,
		PeerType: peerType,
		PeerId:   peerID,
	})
	if err != nil {
		if status.Convert(err).Message() == status.Convert(mtproto.ErrPeerIdInvalid).Message() {
			return false, 0
		}
		return true, 0
	}
	if ext == nil || ext.GetDialog() == nil {
		return false, 0
	}
	return true, ext.GetDialog().GetFolderId().GetValue()
}
