package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/sendas"
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
	if in.GetPeer() != nil && c.MD != nil {
		dest := mtproto.FromInputPeer2(c.MD.UserId, in.GetPeer())
		if dest != nil && !dest.IsEmpty() {
			if choice, ok := sendas.Shared().Get(c.ctx, c.MD.UserId, dest.PeerType, dest.PeerId); ok {
				moveSendAsFront(peers, raw, choice)
			}
		}
	}
	return mtproto.MakeTLChannelsSendAsPeers(&mtproto.Channels_SendAsPeers{
		Peers_VECTORPEER:       raw,
		Peers_VECTORSENDASPEER: peers,
		Chats:                  chats,
		Users:                  []*mtproto.User{},
	}).To_Channels_SendAsPeers(), nil
}

func moveSendAsFront(peers []*mtproto.SendAsPeer, raw []*mtproto.Peer, choice sendas.Choice) {
	match := func(peer *mtproto.Peer) bool {
		if peer == nil {
			return false
		}
		switch choice.Type {
		case mtproto.PEER_USER, mtproto.PEER_SELF:
			return peer.GetUserId() == choice.ID
		case mtproto.PEER_CHANNEL:
			return peer.GetChannelId() == choice.ID
		default:
			return false
		}
	}
	for i := range peers {
		if peers[i] == nil || !match(peers[i].GetPeer()) {
			continue
		}
		peers[0], peers[i] = peers[i], peers[0]
		if i < len(raw) {
			raw[0], raw[i] = raw[i], raw[0]
		}
		return
	}
}
