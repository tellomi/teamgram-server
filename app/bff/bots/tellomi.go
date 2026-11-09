package bots

import (
	"context"
	"strings"
	"time"

	"github.com/teamgram/marmota/pkg/net/rpcx"
	"github.com/teamgram/proto/mtproto"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/pkg/tellomiai"

	"github.com/zeromicro/go-zero/zrpc"
)

// StartTellomi wires replies from user 777001 through messenger.msg.
// TELLOMI_AI unset still installs the sender so a fixed "not open" reply can
// be delivered. The model client is built only when the flag and API key are
// set. A missing API key is logged inside tellomiai.Start and does not stop
// bff. The same sender posts private replies and group replies.
// userConf is the user service, used to resolve the allowlist usernames.
func StartTellomi(msgConf, userConf zrpc.RpcClientConf, dsn string) {
	client := msg_client.NewMsgClient(rpcx.GetCachedRpcClient(msgConf))
	sender := aiSender{client}
	if !tellomiai.Advertised() {
		tellomiai.StartClosed(sender)
		return
	}
	var users tellomiai.UsernameDirectory
	users = userDirectory{c: userclient.NewUserClient(rpcx.GetCachedRpcClient(userConf))}
	tellomiai.Start(dsn, sender, nil, users)
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

func (s aiSender) SendGroup(ctx context.Context, peerType int32, peerID int64, replyTo int32, text string, ai bool) error {
	if s.c == nil {
		return errNoMsg
	}
	_, err := s.c.MsgSendMessageV2(ctx, buildAIGroupMessage(peerType, peerID, replyTo, text, ai, time.Now().UnixNano()))
	return err
}

var errNoMsg = errText("tellomi ai: msg client is nil")

type errText string

func (e errText) Error() string { return string(e) }

// labelEntities bolds a trailing AI mark. Assistant replies no longer end
// with that mark, so they get no entity. Report-review notices still do.
func labelEntities(text string, ai bool) []*mtproto.MessageEntity {
	if !ai || !strings.HasSuffix(text, tellomiai.Label) {
		return nil
	}
	prefix := strings.TrimSuffix(text, tellomiai.Label)
	return []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{
			Offset: int32(mtproto.UTF16Len(prefix)),
			Length: int32(mtproto.UTF16Len(tellomiai.Label)),
		}).To_MessageEntity(),
	}
}

func aiOutbox(peerType int32, peerID int64, text string, ai bool, replyTo int32, random int64) *msg.TLMsgSendMessageV2 {
	var reply *mtproto.MessageReplyHeader
	if replyTo != 0 {
		reply = mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
			ReplyToMsgId:           replyTo,
			ReplyToMsgId_INT32:     replyTo,
			ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(replyTo),
		}).To_MessageReplyHeader()
	}
	out := mtproto.MakeTLMessage(&mtproto.Message{
		Out:      true,
		FromId:   mtproto.MakePeerUser(tellomiai.AccountID),
		PeerId:   mtproto.MakePeer(peerType, peerID),
		ReplyTo:  reply,
		Date:     int32(time.Now().Unix()),
		Message:  text,
		Entities: labelEntities(text, ai),
	}).To_Message()
	return &msg.TLMsgSendMessageV2{
		UserId:   tellomiai.AccountID,
		PeerType: peerType,
		PeerId:   peerID,
		Message: []*msg.OutboxMessage{
			msg.MakeTLOutboxMessage(&msg.OutboxMessage{
				RandomId: random,
				Message:  out,
			}).To_OutboxMessage(),
		},
	}
}

// buildAIMessage is a private-chat message from @tellomi.
func buildAIMessage(to int64, text string, ai bool, random int64) *msg.TLMsgSendMessageV2 {
	return aiOutbox(mtproto.PEER_USER, to, text, ai, 0, random)
}

// buildAIGroupMessage posts into the same group, as a reply to the triggering message.
// msg.sendMessageV2 still requires 777001 to be a member of that group.
func buildAIGroupMessage(peerType int32, peerID int64, replyTo int32, text string, ai bool, random int64) *msg.TLMsgSendMessageV2 {
	return aiOutbox(peerType, peerID, text, ai, replyTo, random)
}
