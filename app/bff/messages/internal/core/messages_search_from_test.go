package core

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/zeromicro/go-zero/core/logx"
)

func TestMessagesSearchRejectsForeignPrivateFromID(t *testing.T) {
	ctx := context.Background()
	c := &MessagesCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 930001},
	}
	_, err := c.MessagesSearch(&mtproto.TLMessagesSearch{
		Peer:   mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 930002}).To_InputPeer(),
		FromId: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 930003}).To_InputPeer(),
		Q:      "apple",
		Limit:  20,
	})
	if err == nil || !strings.Contains(err.Error(), "PEER_ID_INVALID") {
		t.Fatalf("foreign from_id err %v", err)
	}
}

func TestMessagesSearchChannelFromIDMustBeUser(t *testing.T) {
	ctx := context.Background()
	c := &MessagesCore{
		ctx:    ctx,
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 930001},
	}
	_, err := c.MessagesSearch(&mtproto.TLMessagesSearch{
		Peer:   mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 7}).To_InputPeer(),
		FromId: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 9}).To_InputPeer(),
		Q:      "apple",
		Limit:  20,
	})
	if err == nil || !strings.Contains(err.Error(), "FROM_PEER_INVALID") {
		t.Fatalf("channel from_id err %v", err)
	}
}
