package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

func (c *DialogsCore) mergeChannelDialogs(list dialog.DialogExtList) dialog.DialogExtList {
	if c.svcCtx.Plugin == nil {
		return list
	}
	extras, err := c.svcCtx.Plugin.ListChannelDialogs(c.ctx, c.MD.UserId)
	if err != nil || len(extras) == 0 {
		return list
	}
	have := map[int64]bool{}
	for _, ext := range extras {
		if peer := ext.GetDialog().GetPeer(); peer != nil {
			have[peer.GetChannelId()] = true
		}
	}
	out := make(dialog.DialogExtList, 0, len(list)+len(extras))
	for _, ext := range list {
		peer := ext.GetDialog().GetPeer()
		if peer != nil && mtproto.PeerIsChannel(peer) && have[peer.GetChannelId()] {
			continue
		}
		out = append(out, ext)
	}
	return append(out, extras...)
}

func (c *DialogsCore) ensureChannelPeers(list dialog.DialogExtList, peers []*mtproto.PeerUtil) dialog.DialogExtList {
	if c.svcCtx.Plugin == nil {
		return list
	}
	have := map[int64]bool{}
	for _, ext := range list {
		if peer := ext.GetDialog().GetPeer(); peer != nil && mtproto.PeerIsChannel(peer) {
			have[peer.GetChannelId()] = true
		}
	}
	for _, peer := range peers {
		if peer == nil || !peer.IsChannel() || have[peer.PeerId] {
			continue
		}
		ext, err := c.svcCtx.Plugin.GetChannelDialogById(c.ctx, c.MD.UserId, peer.PeerId)
		if err == nil && ext != nil {
			list = append(list, ext)
			have[peer.PeerId] = true
		}
	}
	return list
}

func (c *DialogsCore) channelMessages(ctx context.Context, self int64, ids []dialog.TopMessageId) []*mtproto.Message {
	if c.svcCtx.Plugin == nil {
		return nil
	}
	var out []*mtproto.Message
	for _, id := range ids {
		if !id.Peer.IsChannel() || id.TopMessage == 0 {
			continue
		}
		box, err := c.svcCtx.Plugin.GetChannelMessage(ctx, self, id.Peer.PeerId, id.TopMessage)
		if err == nil && box != nil {
			out = append(out, box.ToMessage(self))
		}
	}
	return out
}

func (c *DialogsCore) channelChats(ctx context.Context, self int64, ids ...int64) []*mtproto.Chat {
	if c.svcCtx.Plugin == nil || len(ids) == 0 {
		return []*mtproto.Chat{}
	}
	return c.svcCtx.Plugin.GetChannelListByIdList(ctx, self, ids...)
}
