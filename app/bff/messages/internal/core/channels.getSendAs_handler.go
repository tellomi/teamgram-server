package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

// ChannelsGetSendAs
// channels.getSendAs#e785a43f flags:# peer:InputPeer = channels.SendAsPeers;
func (c *MessagesCore) ChannelsGetSendAs(in *mtproto.TLChannelsGetSendAs) (*mtproto.Channels_SendAsPeers, error) {
	chats, ids, err := channel.Shared().SendAs(c.ctx, c.MD.UserId)
	if err != nil {
		c.Logger.Errorf("channels.getSendAs - error: %v", err)
		return nil, err
	}
	peers := make([]*mtproto.SendAsPeer, 0, len(ids))
	raw := make([]*mtproto.Peer, 0, len(ids))
	raw = append(raw, mtproto.MakePeerUser(c.MD.UserId))
	peers = append(peers, mtproto.MakeTLSendAsPeer(&mtproto.SendAsPeer{
		Peer: mtproto.MakePeerUser(c.MD.UserId),
	}).To_SendAsPeer())
	for _, chat := range chats {
		raw = append(raw, mtproto.MakePeerChannel(chat.GetId()))
		peers = append(peers, mtproto.MakeTLSendAsPeer(&mtproto.SendAsPeer{
			Peer: mtproto.MakePeerChannel(chat.GetId()),
		}).To_SendAsPeer())
	}
	_ = ids
	return mtproto.MakeTLChannelsSendAsPeers(&mtproto.Channels_SendAsPeers{
		Peers_VECTORPEER:       raw,
		Peers_VECTORSENDASPEER: peers,
		Chats:                  chats,
		Users:                  []*mtproto.User{},
	}).To_Channels_SendAsPeers(), nil
}
