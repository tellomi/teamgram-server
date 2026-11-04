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

func (s announceSender) Send(ctx context.Context, to int64, notice announce.Outbound) error {
	if s.c == nil {
		return errNoMsg
	}
	ok, err := s.c.MsgPushUserMessage(ctx, buildAnnounceMessage(notice, to, randomID()))
	if err != nil {
		return err
	}
	if ok == nil || !mtproto.FromBool(ok) {
		return errors.New("announce: push declined")
	}
	return nil
}

func (s announceSender) Post(ctx context.Context, notice announce.Outbound) error {
	if s.c == nil {
		return errNoMsg
	}
	_, err := s.c.MsgSendMessageV2(ctx, buildNewsPost(notice, randomID()))
	return err
}

// buildAnnounceMessage is a normal private message. PushType 0 is the same
// path as a user-to-user send, including the recipient's push.
func buildAnnounceMessage(notice announce.Outbound, to int64, random int64) *msg.TLMsgPushUserMessage {
	out := mtproto.MakeTLMessage(&mtproto.Message{
		Out:      true,
		FromId:   mtproto.MakePeerUser(notice.From),
		PeerId:   mtproto.MakePeerUser(to),
		Date:     int32(time.Now().Unix()),
		Message:  notice.Text,
		Entities: messageEntities(notice.Entities),
	}).To_Message()
	return &msg.TLMsgPushUserMessage{
		UserId:    notice.From,
		AuthKeyId: 0,
		PeerType:  mtproto.PEER_USER,
		PeerId:    to,
		PushType:  0,
		Message: msg.MakeTLOutboxMessage(&msg.OutboxMessage{
			RandomId: random,
			Message:  out,
		}).To_OutboxMessage(),
	}
}

// buildNewsPost is one channel message in @news from its creator.
func buildNewsPost(notice announce.Outbound, random int64) *msg.TLMsgSendMessageV2 {
	out := mtproto.MakeTLMessage(&mtproto.Message{
		Out:      true,
		Post:     true,
		PeerId:   mtproto.MakePeerChannel(announce.NewsChannelID),
		Date:     int32(time.Now().Unix()),
		Message:  notice.Text,
		Entities: messageEntities(notice.Entities),
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

func messageEntities(ents []announce.Entity) []*mtproto.MessageEntity {
	if len(ents) == 0 {
		return nil
	}
	out := make([]*mtproto.MessageEntity, 0, len(ents))
	for _, e := range ents {
		if e.Type != "bold" {
			continue
		}
		out = append(out, mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{
			Offset: e.Offset,
			Length: e.Length,
		}).To_MessageEntity())
	}
	return out
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
