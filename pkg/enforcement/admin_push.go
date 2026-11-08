package enforcement

import (
	"context"
	"sync"

	"github.com/teamgram/proto/mtproto"
	msgpb "github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
)

// UserMessagePush is the existing messenger client method used by login notices.
type UserMessagePush interface {
	MsgPushUserMessage(ctx context.Context, in *msgpb.TLMsgPushUserMessage) (*mtproto.Bool, error)
}

// AdminSender sends one private message as user 777000. The username string
// is not consulted. A different user named admin does not match this sender.
type AdminSender struct {
	Push UserMessagePush
}

func (a AdminSender) SendFromAdmin(userID int64, key, body string) error {
	if a.Push == nil || userID == 0 || userID == AdminUserID {
		return errNoPush
	}
	_, err := a.Push.MsgPushUserMessage(context.Background(), &msgpb.TLMsgPushUserMessage{
		UserId:   AdminUserID,
		PeerType: mtproto.PEER_USER,
		PeerId:   userID,
		PushType: 1,
		Message: msgpb.MakeTLOutboxMessage(&msgpb.OutboxMessage{
			RandomId: int64(len(key) + len(body)),
			Message: mtproto.MakeTLMessage(&mtproto.Message{
				Out:     true,
				FromId:  mtproto.MakePeerUser(AdminUserID),
				PeerId:  mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: userID}).To_Peer(),
				Message: body,
			}).To_Message(),
		}).To_OutboxMessage(),
	})
	return err
}

var errNoPush = errString("enforcement: admin sender is not installed")

type errString string

func (e errString) Error() string { return string(e) }

// Inbox is an isolated stand-in for the 777000 outbox consumer.
type Inbox struct {
	mu   sync.Mutex
	Fail int
	Rows []InboxRow
}

type InboxRow struct {
	From int64
	To   int64
	Key  string
	Body string
}

func (n *Inbox) SendFromAdmin(userID int64, key, body string) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.Fail > 0 {
		n.Fail--
		return errNoPush
	}
	n.Rows = append(n.Rows, InboxRow{From: AdminUserID, To: userID, Key: key, Body: body})
	return nil
}

func (n *Inbox) Count() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.Rows)
}
