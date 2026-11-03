package service

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TodoListsService serves checklist append and completion toggles.
// https://core.telegram.org/api/todo
//
// The checklist lives in the message as messageMediaToDo. These RPCs edit
// that media and post the service message official clients expect
// (messageActionTodoAppendTasks / messageActionTodoCompletions).
type TodoListsService struct {
	mtproto.UnimplementedRPCTodoListsServer
	Msg     msg_client.MsgClient
	Message message_client.MessageClient
}

func (s TodoListsService) MessagesAppendTodoList(ctx context.Context, in *mtproto.TLMessagesAppendTodoList) (*mtproto.Updates, error) {
	return s.changeTodo(ctx, in.GetPeer(), in.GetMsgId(), true, func(media *mtproto.MessageMedia) (*mtproto.MessageAction, error) {
		added, err := mtproto.AppendTodoItems(media, in.GetList())
		if err != nil {
			return nil, err
		}
		return mtproto.MakeTLMessageActionTodoAppendTasks(&mtproto.MessageAction{List: added}).To_MessageAction(), nil
	})
}

func (s TodoListsService) MessagesToggleTodoCompleted(ctx context.Context, in *mtproto.TLMessagesToggleTodoCompleted) (*mtproto.Updates, error) {
	return s.changeTodo(ctx, in.GetPeer(), in.GetMsgId(), false, func(media *mtproto.MessageMedia) (*mtproto.MessageAction, error) {
		self, _ := selfID(ctx)
		done, undone, err := mtproto.ToggleTodoItems(media, self, in.GetCompleted(), in.GetIncompleted(), int32(time.Now().Unix()))
		if err != nil {
			return nil, err
		}
		return mtproto.MakeTLMessageActionTodoCompletions(&mtproto.MessageAction{
			Completed:   done,
			Incompleted: undone,
		}).To_MessageAction(), nil
	})
}

func (s TodoListsService) changeTodo(ctx context.Context, peerIn *mtproto.InputPeer, msgID int32, appendItems bool, apply func(*mtproto.MessageMedia) (*mtproto.MessageAction, error)) (*mtproto.Updates, error) {
	self, auth := selfID(ctx)
	if self == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if msgID <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	peer := mtproto.FromInputPeer2(self, peerIn)
	if peer.PeerType == mtproto.PEER_SELF {
		peer.PeerType = mtproto.PEER_USER
		if peer.PeerId == 0 {
			peer.PeerId = self
		}
	}
	switch peer.PeerType {
	case mtproto.PEER_USER, mtproto.PEER_CHAT, mtproto.PEER_CHANNEL:
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}

	var (
		author int64
		body   *mtproto.Message
		box    *mtproto.MessageBox
		err    error
	)
	if peer.IsChannel() {
		author, body, err = channel.Shared().LoadMessageAuthor(ctx, peer.PeerId, msgID)
	} else {
		box, err = s.loadBox(ctx, self, msgID)
		if err == nil {
			author = box.GetSenderUserId()
			if author == 0 {
				author = box.GetMessage().GetFromId().GetUserId()
			}
			body = proto.Clone(box.GetMessage()).(*mtproto.Message)
		}
	}
	if err != nil {
		return nil, err
	}
	if body.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaToDo || body.GetMedia().GetTodo() == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	others := body.GetMedia().GetTodo().GetOthersCanComplete()
	if appendItems {
		others = body.GetMedia().GetTodo().GetOthersCanAppend()
	}
	if !mtproto.TodoActorAllowed(author, self, others) {
		return nil, mtproto.ErrMessageAuthorRequired
	}
	action, err := apply(body.GetMedia())
	if err != nil {
		return nil, err
	}

	var edited *mtproto.Updates
	if peer.IsChannel() {
		edited, err = channel.Shared().RewriteBody(ctx, self, peer.PeerId, auth, msgID, body)
	} else {
		body.EditDate = &wrapperspb.Int32Value{Value: int32(time.Now().Unix())}
		edited, err = s.Msg.MsgEditMessageV2(ctx, &msg.TLMsgEditMessageV2{
			UserId:    self,
			AuthKeyId: auth,
			PeerType:  peer.PeerType,
			PeerId:    peer.PeerId,
			EditType:  msg.EditTypeNormal,
			NewMessage: msg.MakeTLOutboxMessage(&msg.OutboxMessage{
				NoWebpage: true,
				Message:   body,
			}).To_OutboxMessage(),
			DstMessage: box,
		})
	}
	if err != nil {
		return nil, err
	}
	sent, err := s.sendTodoAction(ctx, self, auth, peer, action)
	if err != nil {
		return nil, err
	}
	return joinUpdates(edited, sent), nil
}

func (s TodoListsService) loadBox(ctx context.Context, self int64, msgID int32) (*mtproto.MessageBox, error) {
	if s.Message == nil {
		return nil, mtproto.ErrInternalServerError
	}
	box, err := s.Message.MessageGetUserMessage(ctx, &messagepb.TLMessageGetUserMessage{
		UserId: self,
		Id:     msgID,
	})
	if err != nil {
		return nil, err
	}
	if box == nil || box.GetMessage() == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return box, nil
}

func (s TodoListsService) sendTodoAction(ctx context.Context, self, auth int64, peer *mtproto.PeerUtil, action *mtproto.MessageAction) (*mtproto.Updates, error) {
	message := mtproto.MakeTLMessageService(&mtproto.Message{
		Out:    true,
		FromId: mtproto.MakePeerUser(self),
		PeerId: peer.ToPeer(),
		Date:   int32(time.Now().Unix()),
		Action: action,
	}).To_Message()
	if peer.IsSelfUser(self) {
		message.SavedPeerId = mtproto.MakePeerUser(self)
	}
	if peer.IsChannel() {
		return channel.Shared().Send(ctx, self, peer.PeerId, auth, []channel.Outgoing{{
			RandomID: randID(),
			Message:  message,
		}})
	}
	if s.Msg == nil {
		return nil, mtproto.ErrInternalServerError
	}
	return s.Msg.MsgSendMessageV2(ctx, &msg.TLMsgSendMessageV2{
		UserId:    self,
		AuthKeyId: auth,
		PeerType:  peer.PeerType,
		PeerId:    peer.PeerId,
		Message: []*msg.OutboxMessage{
			msg.MakeTLOutboxMessage(&msg.OutboxMessage{
				NoWebpage: true,
				RandomId:  randID(),
				Message:   message,
			}).To_OutboxMessage(),
		},
	})
}

func joinUpdates(parts ...*mtproto.Updates) *mtproto.Updates {
	base := parts[0]
	if base == nil {
		base = mtproto.MakeEmptyUpdates()
	}
	for _, extra := range parts[1:] {
		if extra == nil {
			continue
		}
		base.Updates = append(base.Updates, extra.Updates...)
		base.Users = append(base.Users, extra.Users...)
		base.Chats = append(base.Chats, extra.Chats...)
	}
	if base.Updates == nil {
		base.Updates = []*mtproto.Update{}
	}
	if base.Users == nil {
		base.Users = []*mtproto.User{}
	}
	if base.Chats == nil {
		base.Chats = []*mtproto.Chat{}
	}
	return base
}
