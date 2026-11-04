package bots

import (
	"context"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"

	"github.com/zeromicro/go-zero/zrpc"
)

// StartTellomi wires private replies from user 777001 through messenger.msg.
// TELLOMI_AI unset returns before any client is built. A missing API key is
// logged inside tellomiai.Start and does not stop bff.
func StartTellomi(conf zrpc.RpcClientConf, dsn string) {
	if !tellomiai.Advertised() {
		return
	}
	client := msg_client.NewMsgClient(rpcx.GetCachedRpcClient(conf))
	tellomiai.Start(dsn, aiSender{client}, nil)
}

type aiSender struct {
	c msg_client.MsgClient
}

func (s aiSender) Send(ctx context.Context, to int64, text string, ai bool) error {
	if s.c == nil {
		return errNoMsg
	}
	_, err := s.c.MsgSendMessageV2(ctx, buildAIMessage(to, text, ai, time.Now().UnixNano()))
	return err
}

var errNoMsg = errText("tellomi ai: msg client is nil")

type errText string

func (e errText) Error() string { return string(e) }

// buildAIMessage is a private-chat message from @tellomi. When ai is set and
// the text ends with the visible label, that label is messageEntityBold.
func buildAIMessage(to int64, text string, ai bool, random int64) *msg.TLMsgSendMessageV2 {
	var entities []*mtproto.MessageEntity
	if ai && strings.HasSuffix(text, tellomiai.Label) {
		prefix := strings.TrimSuffix(text, tellomiai.Label)
		entities = []*mtproto.MessageEntity{
			mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{
				Offset: int32(mtproto.UTF16Len(prefix)),
				Length: int32(mtproto.UTF16Len(tellomiai.Label)),
			}).To_MessageEntity(),
		}
	}
	out := mtproto.MakeTLMessage(&mtproto.Message{
		Out:      true,
		FromId:   mtproto.MakePeerUser(tellomiai.AccountID),
		PeerId:   mtproto.MakePeerUser(to),
		Date:     int32(time.Now().Unix()),
		Message:  text,
		Entities: entities,
	}).To_Message()
	return &msg.TLMsgSendMessageV2{
		UserId:   tellomiai.AccountID,
		PeerType: mtproto.PEER_USER,
		PeerId:   to,
		Message: []*msg.OutboxMessage{
			msg.MakeTLOutboxMessage(&msg.OutboxMessage{
				RandomId: random,
				Message:  out,
			}).To_OutboxMessage(),
		},
	}
}
