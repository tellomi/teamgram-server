package bots

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"time"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/pkg/announce"

	"github.com/zeromicro/go-zero/zrpc"
)

// StartAnnounce wires the launch-notice listener. TELLOMI_ANNOUNCE unset
// returns before any client is built. It is not on the login path.
func StartAnnounce(conf zrpc.RpcClientConf, dsn string) {
	if !announce.Advertised() {
		return
	}
	client := msgclient.NewMsgClient(rpcx.GetCachedRpcClient(conf))
	s := announceSender{c: client}
	announce.Start(dsn, s, s)
}

type announceSender struct {
	c msgclient.MsgClient
}

func (s announceSender) Send(ctx context.Context, to int64, text string) error {
	if s.c == nil {
		return errNoMsg
	}
	ok, err := s.c.MsgPushUserMessage(ctx, buildAnnounceMessage(to, text, randomID()))
	if err != nil {
		return err
	}
	if ok == nil || !mtproto.FromBool(ok) {
		return errors.New("announce: push declined")
	}
	return nil
}

func (s announceSender) Post(ctx context.Context, text string) error {
	if s.c == nil {
		return errNoMsg
	}
	_, err := s.c.MsgSendMessageV2(ctx, buildNewsPost(text, randomID()))
	return err
}

// buildAnnounceMessage is a private message from @admin, the same shape as
// a login-code notice (push type 1) without a spoiler.
func buildAnnounceMessage(to int64, text string, random int64) *msg.TLMsgPushUserMessage {
	out := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		FromId:  mtproto.MakePeerUser(announce.ServiceUserID),
		PeerId:  mtproto.MakePeerUser(to),
		Date:    int32(time.Now().Unix()),
		Message: text,
	}).To_Message()
	return &msg.TLMsgPushUserMessage{
		UserId:    announce.ServiceUserID,
		AuthKeyId: 0,
		PeerType:  mtproto.PEER_USER,
		PeerId:    to,
		PushType:  1,
		Message: msg.MakeTLOutboxMessage(&msg.OutboxMessage{
			RandomId: random,
			Message:  out,
		}).To_OutboxMessage(),
	}
}

// buildNewsPost is one channel message in @news from its creator.
func buildNewsPost(text string, random int64) *msg.TLMsgSendMessageV2 {
	out := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		Post:    true,
		PeerId:  mtproto.MakePeerChannel(announce.NewsChannelID),
		Date:    int32(time.Now().Unix()),
		Message: text,
	}).To_Message()
	return &msg.TLMsgSendMessageV2{
		UserId:   announce.NewsPosterID,
		PeerType: mtproto.PEER_CHANNEL,
		PeerId:   announce.NewsChannelID,
		Message: []*msg.OutboxMessage{
			msg.MakeTLOutboxMessage(&msg.OutboxMessage{
				RandomId: random,
				Message:  out,
			}).To_OutboxMessage(),
		},
	}
}

func randomID() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return time.Now().UnixNano()
	}
	v := int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}
