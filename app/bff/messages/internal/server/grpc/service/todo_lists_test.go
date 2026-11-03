package service

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	msg_client "github.com/teamgram/teamgram-server/app/messenger/msg/msg/client"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	messagepb "github.com/teamgram/teamgram-server/app/service/biz/message/message"

	"github.com/zeromicro/go-zero/core/jsonx"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func withUser(userID int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: userID + 10}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

type todoStore struct {
	box    *mtproto.MessageBox
	edited *mtproto.Message
	sent   *mtproto.Message
}

type todoMessages struct {
	message_client.MessageClient
	store *todoStore
}

func (m todoMessages) MessageGetUserMessage(_ context.Context, in *messagepb.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	if m.store == nil || m.store.box == nil || in.GetId() != m.store.box.GetMessageId() {
		return nil, nil
	}
	return m.store.box, nil
}

type todoMsg struct {
	msg_client.MsgClient
	store *todoStore
}

func (m todoMsg) MsgEditMessageV2(_ context.Context, in *msg.TLMsgEditMessageV2) (*mtproto.Updates, error) {
	m.store.edited = in.GetNewMessage().GetMessage()
	if m.store.box != nil {
		m.store.box.Message = m.store.edited
	}
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateEditMessage(&mtproto.Update{
		Message_MESSAGE: m.store.edited,
		Pts_INT32:       10,
		PtsCount:        1,
	}).To_Update()), nil
}

func (m todoMsg) MsgSendMessageV2(_ context.Context, in *msg.TLMsgSendMessageV2) (*mtproto.Updates, error) {
	if len(in.GetMessage()) == 0 {
		return nil, mtproto.ErrMessageEmpty
	}
	m.store.sent = in.GetMessage()[0].GetMessage()
	return mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateNewMessage(&mtproto.Update{
		Message_MESSAGE: m.store.sent,
		Pts_INT32:       11,
		PtsCount:        1,
	}).To_Update()), nil
}

func checklistBox(author int64, othersComplete, othersAppend bool) *mtproto.MessageBox {
	media, err := mtproto.MessageMediaFromTodo(mtproto.MakeTLTodoList(&mtproto.TodoList{
		OthersCanAppend:   othersAppend,
		OthersCanComplete: othersComplete,
		Title:             mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "购物"}).To_TextWithEntities(),
		List: []*mtproto.TodoItem{
			mtproto.MakeTLTodoItem(&mtproto.TodoItem{
				Id:    1,
				Title: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "牛奶"}).To_TextWithEntities(),
			}).To_TodoItem(),
			mtproto.MakeTLTodoItem(&mtproto.TodoItem{
				Id:    2,
				Title: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "面包"}).To_TextWithEntities(),
			}).To_TodoItem(),
		},
	}).To_TodoList())
	if err != nil {
		panic(err)
	}
	return &mtproto.MessageBox{
		UserId:       author,
		MessageId:    4,
		SenderUserId: author,
		PeerType:     mtproto.PEER_USER,
		PeerId:       20,
		Message: mtproto.MakeTLMessage(&mtproto.Message{
			Id:     4,
			FromId: mtproto.MakePeerUser(author),
			PeerId: mtproto.MakePeerUser(20),
			Media:  media,
		}).To_Message(),
	}
}

func TestTodoUserToggleAndAppend(t *testing.T) {
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 20}).To_InputPeer()
	store := &todoStore{box: checklistBox(1, true, true)}
	svc := TodoListsService{Msg: todoMsg{store: store}, Message: todoMessages{store: store}}
	ups, err := svc.MessagesToggleTodoCompleted(withUser(2), &mtproto.TLMessagesToggleTodoCompleted{
		Peer:      peer,
		MsgId:     4,
		Completed: []int32{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if store.edited.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaToDo {
		t.Fatalf("edited media %s", store.edited.GetMedia().GetPredicateName())
	}
	if len(store.edited.GetMedia().GetCompletions()) != 1 || store.edited.GetMedia().GetCompletions()[0].GetCompletedBy_PEER().GetUserId() != 2 {
		t.Fatalf("completion %+v", store.edited.GetMedia().GetCompletions())
	}
	if store.edited.GetMedia().GetTodo().GetList()[1].GetTitle().GetText() != "面包" {
		t.Fatal("items dropped")
	}
	if store.sent.GetAction().GetPredicateName() != mtproto.Predicate_messageActionTodoCompletions || len(store.sent.GetAction().GetCompleted()) != 1 {
		t.Fatalf("service %+v", store.sent.GetAction())
	}
	var sawEdit, sawNew bool
	for _, u := range ups.GetUpdates() {
		if u.GetPredicateName() == mtproto.Predicate_updateEditMessage {
			sawEdit = true
		}
		if u.GetPredicateName() == mtproto.Predicate_updateNewMessage && u.GetMessage_MESSAGE().GetAction().GetPredicateName() == mtproto.Predicate_messageActionTodoCompletions {
			sawNew = true
		}
	}
	if !sawEdit || !sawNew {
		t.Fatalf("updates %+v", ups.GetUpdates())
	}

	// The row getHistory reads back is this JSON. It must still be messageMediaToDo.
	raw, err := jsonx.Marshal(store.edited)
	if err != nil {
		t.Fatal(err)
	}
	var stored mtproto.Message
	if err := jsonx.Unmarshal(raw, &stored); err != nil {
		t.Fatal(err)
	}
	buf := mtproto.NewEncodeBuf(2048)
	if err := stored.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	got := &mtproto.Message{}
	if err := got.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	if uint32(got.GetMedia().GetConstructor()) != 0x8a53b014 ||
		uint32(got.GetMedia().GetTodo().GetConstructor()) != 0x49b92a26 ||
		uint32(got.GetMedia().GetTodo().GetList()[0].GetConstructor()) != 0xcba9a52f ||
		uint32(got.GetMedia().GetTodo().GetTitle().GetConstructor()) != 0x751f3146 ||
		uint32(got.GetMedia().GetCompletions()[0].GetConstructor()) != 0x221bb5e4 {
		t.Fatalf("refetch media 0x%x list 0x%x item 0x%x text 0x%x completion 0x%x",
			uint32(got.GetMedia().GetConstructor()),
			uint32(got.GetMedia().GetTodo().GetConstructor()),
			uint32(got.GetMedia().GetTodo().GetList()[0].GetConstructor()),
			uint32(got.GetMedia().GetTodo().GetTitle().GetConstructor()),
			uint32(got.GetMedia().GetCompletions()[0].GetConstructor()))
	}

	item := mtproto.MakeTLTodoItem(&mtproto.TodoItem{
		Id:    3,
		Title: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "鸡蛋"}).To_TextWithEntities(),
	}).To_TodoItem()
	if _, err := svc.MessagesAppendTodoList(withUser(2), &mtproto.TLMessagesAppendTodoList{
		Peer: peer, MsgId: 4, List: []*mtproto.TodoItem{item},
	}); err != nil {
		t.Fatal(err)
	}
	if len(store.edited.GetMedia().GetTodo().GetList()) != 3 || store.edited.GetMedia().GetCompletions()[0].GetId() != 1 {
		t.Fatalf("append lost state %+v", store.edited.GetMedia())
	}
	if store.sent.GetAction().GetPredicateName() != mtproto.Predicate_messageActionTodoAppendTasks || store.sent.GetAction().GetList()[0].GetId() != 3 {
		t.Fatalf("append action %+v", store.sent.GetAction())
	}
	if _, err := svc.MessagesAppendTodoList(withUser(2), &mtproto.TLMessagesAppendTodoList{
		Peer: peer, MsgId: 4, List: []*mtproto.TodoItem{item},
	}); status.Convert(err).Message() != "TODO_ITEM_DUPLICATE" {
		t.Fatalf("dup %v", err)
	}
}

func TestTodoAuthorRequired(t *testing.T) {
	peer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 20}).To_InputPeer()
	store := &todoStore{box: checklistBox(1, false, false)}
	svc := TodoListsService{Msg: todoMsg{store: store}, Message: todoMessages{store: store}}
	if _, err := svc.MessagesToggleTodoCompleted(withUser(2), &mtproto.TLMessagesToggleTodoCompleted{
		Peer: peer, MsgId: 4, Completed: []int32{1},
	}); status.Convert(err).Message() != "MESSAGE_AUTHOR_REQUIRED" {
		t.Fatalf("toggle %v", err)
	}
	item := mtproto.MakeTLTodoItem(&mtproto.TodoItem{
		Id:    3,
		Title: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "鸡蛋"}).To_TextWithEntities(),
	}).To_TodoItem()
	if _, err := svc.MessagesAppendTodoList(withUser(2), &mtproto.TLMessagesAppendTodoList{
		Peer: peer, MsgId: 4, List: []*mtproto.TodoItem{item},
	}); status.Convert(err).Message() != "MESSAGE_AUTHOR_REQUIRED" {
		t.Fatalf("append %v", err)
	}
	if _, err := svc.MessagesToggleTodoCompleted(withUser(1), &mtproto.TLMessagesToggleTodoCompleted{
		Peer: peer, MsgId: 0, Completed: []int32{1},
	}); status.Convert(err).Message() != "MESSAGE_ID_INVALID" {
		t.Fatalf("id %v", err)
	}
}

func TestTodoChannelHistoryKeepsMedia(t *testing.T) {
	prev := channel.Shared()
	chSvc := channel.New(channel.NewMemoryStore())
	channel.Use(chSvc)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := context.Background()
	up, err := chSvc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "清单", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	ch := up.Chats[0]
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ch.Id,
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
	if _, err := chSvc.Invite(ctx, 1, 0, in, []int64{2}, nil); err != nil {
		t.Fatal(err)
	}
	media, err := mtproto.MessageMediaFromTodo(mtproto.MakeTLTodoList(&mtproto.TodoList{
		OthersCanComplete: true,
		Title:             mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "购物"}).To_TextWithEntities(),
		List: []*mtproto.TodoItem{
			mtproto.MakeTLTodoItem(&mtproto.TodoItem{
				Id:    1,
				Title: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "牛奶"}).To_TextWithEntities(),
			}).To_TodoItem(),
		},
	}).To_TodoList())
	if err != nil {
		t.Fatal(err)
	}
	sent, err := chSvc.Send(ctx, 1, ch.Id, 0, []channel.Outgoing{{
		RandomID: 11,
		Message: mtproto.MakeTLMessage(&mtproto.Message{
			Date: 5, Message: "买", Media: media,
		}).To_Message(),
	}})
	if err != nil {
		t.Fatal(err)
	}
	msgID := sent.Updates[0].GetMessage_MESSAGE().GetId()
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: ch.Id}).To_InputPeer()
	svc := TodoListsService{}
	ups, err := svc.MessagesToggleTodoCompleted(withUser(2), &mtproto.TLMessagesToggleTodoCompleted{
		Peer: peer, MsgId: msgID, Completed: []int32{1},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := chSvc.LoadMessage(ctx, ch.Id, msgID)
	if err != nil {
		t.Fatal(err)
	}
	// Channel rows are protobuf. Unmarshal the way getHistory does, then encode at layer 232.
	raw, err := proto.Marshal(stored)
	if err != nil {
		t.Fatal(err)
	}
	var again mtproto.Message
	if err := proto.Unmarshal(raw, &again); err != nil {
		t.Fatal(err)
	}
	buf := mtproto.NewEncodeBuf(2048)
	if err := again.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	got := &mtproto.Message{}
	if err := got.Decode(mtproto.NewDecodeBuf(buf.GetBuf())); err != nil {
		t.Fatal(err)
	}
	if got.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaToDo ||
		uint32(got.GetMedia().GetConstructor()) != 0x8a53b014 ||
		uint32(got.GetMedia().GetTodo().GetConstructor()) != 0x49b92a26 ||
		uint32(got.GetMedia().GetTodo().GetList()[0].GetConstructor()) != 0xcba9a52f ||
		uint32(got.GetMedia().GetTodo().GetTitle().GetConstructor()) != 0x751f3146 ||
		uint32(got.GetMedia().GetCompletions()[0].GetConstructor()) != 0x221bb5e4 {
		t.Fatalf("refetch media 0x%x list 0x%x item 0x%x text 0x%x completion 0x%x",
			uint32(got.GetMedia().GetConstructor()),
			uint32(got.GetMedia().GetTodo().GetConstructor()),
			uint32(got.GetMedia().GetTodo().GetList()[0].GetConstructor()),
			uint32(got.GetMedia().GetTodo().GetTitle().GetConstructor()),
			uint32(got.GetMedia().GetCompletions()[0].GetConstructor()))
	}
	if len(got.GetMedia().GetCompletions()) != 1 || got.GetMedia().GetTodo().GetList()[0].GetTitle().GetText() != "牛奶" {
		t.Fatalf("stored %+v", got.GetMedia())
	}
	if got.GetMedia().GetCompletions()[0].GetCompletedBy_PEER().GetUserId() != 2 {
		t.Fatalf("completed_by %+v", got.GetMedia().GetCompletions()[0])
	}
	var serviceOK bool
	for _, u := range ups.GetUpdates() {
		if u.GetMessage_MESSAGE().GetAction().GetPredicateName() == mtproto.Predicate_messageActionTodoCompletions {
			serviceOK = true
		}
	}
	if !serviceOK {
		t.Fatalf("channel updates %+v", ups.GetUpdates())
	}

	if _, err := svc.MessagesAppendTodoList(withUser(2), &mtproto.TLMessagesAppendTodoList{
		Peer: peer, MsgId: msgID, List: []*mtproto.TodoItem{
			mtproto.MakeTLTodoItem(&mtproto.TodoItem{
				Id:    2,
				Title: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{Text: "面包"}).To_TextWithEntities(),
			}).To_TodoItem(),
		},
	}); status.Convert(err).Message() != "MESSAGE_AUTHOR_REQUIRED" {
		t.Fatalf("append %v", err)
	}
}
