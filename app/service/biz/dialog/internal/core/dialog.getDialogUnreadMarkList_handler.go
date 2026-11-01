package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// DialogGetDialogUnreadMarkList
// dialog.getDialogUnreadMarkList user_id:long = Vector<DialogPeer>;
func (c *DialogCore) DialogGetDialogUnreadMarkList(in *dialog.TLDialogGetDialogUnreadMarkList) (*dialog.Vector_DialogPeer, error) {
	rows, err := c.svcCtx.Dao.ListUnreadMarks(c.ctx, in.GetUserId())
	if err != nil {
		c.Logger.Errorf("dialog.getDialogUnreadMarkList - error: %v", err)
		return nil, err
	}
	out := &dialog.Vector_DialogPeer{Datas: make([]*mtproto.DialogPeer, 0, len(rows))}
	for _, row := range rows {
		var peer *mtproto.Peer
		switch row.PeerType {
		case mtproto.PEER_SELF, mtproto.PEER_USER:
			peer = mtproto.MakePeerUser(row.PeerId)
		case mtproto.PEER_CHAT:
			peer = mtproto.MakePeerChat(row.PeerId)
		case mtproto.PEER_CHANNEL:
			peer = mtproto.MakePeerChannel(row.PeerId)
		default:
			continue
		}
		out.Datas = append(out.Datas, mtproto.MakeTLDialogPeer(&mtproto.DialogPeer{Peer: peer}).To_DialogPeer())
	}
	return out, nil
}
