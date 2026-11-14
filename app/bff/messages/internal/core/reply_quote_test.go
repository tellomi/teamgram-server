package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	messageclient "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestQuoteTextMustOccurInTheOriginalMessage(t *testing.T) {
	if !quoteTextValid("hello world", "world", wrapperspb.Int32(6)) {
		t.Fatal("plain slice")
	}
	if quoteTextValid("hello world", "world", wrapperspb.Int32(0)) {
		t.Fatal("offset 0 is not world")
	}
	if !quoteTextValid("hello 😀", "😀", wrapperspb.Int32(6)) {
		t.Fatal("emoji is two UTF-16 units and starts at 6")
	}
	if quoteTextValid("hello 😀", "😀", wrapperspb.Int32(7)) {
		t.Fatal("offset inside the emoji")
	}
	if quoteTextValid("hello", "", nil) {
		t.Fatal("empty quote")
	}

	msgs := &quoteMsgs{box: &mtproto.MessageBox{Message: &mtproto.Message{Message: "buy milk today"}}}
	c := &MessagesCore{
		ctx:    context.Background(),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{MessageClient: msgs}},
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	in := mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: 4,
		QuoteText:    wrapperspb.String("milk"),
		QuoteOffset:  wrapperspb.Int32(4),
		TodoItemId:   wrapperspb.Int32(2),
	}).To_InputReplyTo()
	h, err := c.replyHeaderChecked(mtproto.MakePeerUtil(mtproto.PEER_USER, 9), in)
	if err != nil || h.GetQuoteText().GetValue() != "milk" || h.GetTodoItemId().GetValue() != 2 {
		t.Fatalf("kept %+v %v", h, err)
	}
	bad := mtproto.MakeTLInputReplyToMessage(&mtproto.InputReplyTo{
		ReplyToMsgId: 4,
		QuoteText:    wrapperspb.String("bread"),
	}).To_InputReplyTo()
	if _, err = c.replyHeaderChecked(mtproto.MakePeerUtil(mtproto.PEER_USER, 9), bad); status.Convert(err).Message() != "QUOTE_TEXT_INVALID" {
		t.Fatalf("invalid quote: %v", err)
	}
}

type quoteMsgs struct {
	messageclient.MessageClient
	box *mtproto.MessageBox
}

func (q *quoteMsgs) MessageGetUserMessage(context.Context, *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	return q.box, nil
}
