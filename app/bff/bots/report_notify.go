package bots

import (
	"context"
	"time"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	msgclient "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/report"

	"github.com/zeromicro/go-zero/zrpc"
)

// StartReports wires the unified report table, the @admin receipt, and the
// optional review listener. An empty DSN keeps rows in process memory.
// Login and session handling are not on this path.
func StartReports(conf zrpc.RpcClientConf, dsn string) {
	client := msgclient.NewMsgClient(rpcx.GetCachedRpcClient(conf))
	report.Start(dsn, reportSender{c: client})
}

type reportSender struct {
	c msgclient.MsgClient
}

func (s reportSender) Send(ctx context.Context, to int64, text string) error {
	if s.c == nil {
		return errNoMsg
	}
	ok, err := s.c.MsgPushUserMessage(ctx, buildReportMessage(to, text, randomID()))
	if err != nil {
		return err
	}
	if ok == nil || !mtproto.FromBool(ok) {
		return errText("report: push declined")
	}
	return nil
}

// buildReportMessage is a private message from @admin. It carries no model mark.
func buildReportMessage(to int64, text string, random int64) *msg.TLMsgPushUserMessage {
	out := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		FromId:  mtproto.MakePeerUser(report.AdminUserID),
		PeerId:  mtproto.MakePeerUser(to),
		Date:    int32(time.Now().Unix()),
		Message: text,
	}).To_Message()
	return &msg.TLMsgPushUserMessage{
		UserId:    report.AdminUserID,
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
