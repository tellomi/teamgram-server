package service

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

// TestRepliesNoticeFeedsBlockFromReplies stores the notice in the caller's
// private inbox and uses the existing handler. The same numeric id in another
// account is not the @replies row, and the discussion message id is not
// accepted unless it is that inbox id.
func TestRepliesNoticeFeedsBlockFromReplies(t *testing.T) {
	t.Setenv("TELLOMI_CHANNEL_COMMENTS", "1")
	if channel.RepliesUserID != repliesUserID {
		t.Fatalf("replies user %d", channel.RepliesUserID)
	}
	svc := channel.New(channel.NewMemoryStore())
	boxes := map[int64]map[int32]*mtproto.MessageBox{}
	var blocked int64
	var writes int
	svc.SetHooks(channel.Hooks{RepliesInbox: func(_ context.Context, n channel.RepliesNotice) error {
		pol := channel.RepliesDeliveryOf(blocked == n.Commenter && n.Recipient == 3, 100)
		if !pol.Write {
			return nil
		}
		writes++
		msg := proto.Clone(n.Message).(*mtproto.Message)
		msg.Id = 50
		msg.FromId = nil
		msg.PeerId = mtproto.MakePeerUser(repliesUserID)
		if boxes[n.Recipient] == nil {
			boxes[n.Recipient] = map[int32]*mtproto.MessageBox{}
		}
		boxes[n.Recipient][50] = &mtproto.MessageBox{
			UserId:       n.Recipient,
			MessageId:    50,
			SenderUserId: repliesUserID,
			PeerType:     mtproto.PEER_USER,
			PeerId:       repliesUserID,
			Message:      msg,
		}
		return nil
	}})
	ctx := context.Background()
	news, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "News", Broadcast: true})
	if err != nil {
		t.Fatal(err)
	}
	room, err := svc.Create(ctx, channel.CreateRequest{CreatorID: 1, Title: "Talk", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	group := room.Chats[0]
	in := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  group.Id,
		AccessHash: group.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
	nin := mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  news.Chats[0].Id,
		AccessHash: news.Chats[0].GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
	if err = svc.SetDiscussionGroup(ctx, 1, 0, nin, in); err != nil {
		t.Fatal(err)
	}
	if _, err = svc.Invite(ctx, 1, 0, in, []int64{3, 4, 8}, nil); err != nil {
		t.Fatal(err)
	}
	post, err := svc.Send(ctx, 1, news.Chats[0].Id, 0, []channel.Outgoing{{
		RandomID: 1,
		Message:  &mtproto.Message{Message: "post", Date: 5},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var postID int32
	for _, u := range post.GetUpdates() {
		if id := u.GetMessage_MESSAGE().GetId(); id > postID {
			postID = id
		}
	}
	comment, err := svc.Send(ctx, 3, news.Chats[0].Id, 0, []channel.Outgoing{{
		RandomID: 2,
		Message: &mtproto.Message{
			Message: "parent",
			Date:    6,
			ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
				ReplyToMsgId:           postID,
				ReplyToMsgId_INT32:     postID,
				ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(postID),
			}).To_MessageReplyHeader(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var parentID int32
	for _, u := range comment.GetUpdates() {
		if id := u.GetMessage_MESSAGE().GetId(); id > parentID {
			parentID = id
		}
	}
	reply, err := svc.Send(ctx, 8, group.Id, 0, []channel.Outgoing{{
		RandomID: 3,
		Message: &mtproto.Message{
			Message: "from-eight",
			Date:    7,
			ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
				ReplyToMsgId:           parentID,
				ReplyToMsgId_INT32:     parentID,
				ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(parentID),
			}).To_MessageReplyHeader(),
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var commentID int32
	for _, u := range reply.GetUpdates() {
		if id := u.GetMessage_MESSAGE().GetId(); id > commentID {
			commentID = id
		}
	}
	if boxes[3][50] == nil || boxes[3][50].GetMessage().GetFwdFrom().GetFromId().GetUserId() != 8 {
		t.Fatalf("inbox %#v", boxes[3][50])
	}
	if boxes[3][50].GetMessageId() == commentID && commentID == 50 {
		t.Fatal("fixture failed to separate the inbox id from the channel id")
	}
	boxes[4] = map[int32]*mtproto.MessageBox{
		50: {
			UserId:       4,
			MessageId:    50,
			SenderUserId: 9,
			PeerType:     mtproto.PEER_USER,
			PeerId:       9,
			Message:      &mtproto.Message{Id: 50, Message: "other-account"},
		},
	}
	s := ThreadsService{
		Messages: noticeInbox{rows: boxes},
		Users:    repliesUserFake{blocked: &blocked},
	}
	if _, err = s.ContactsBlockFromReplies(noticeCtx(4), &mtproto.TLContactsBlockFromReplies{MsgId: 50}); status.Convert(err).Message() != "MSG_ID_INVALID" || blocked != 0 {
		t.Fatalf("other account err %v blocked %d", err, blocked)
	}
	if _, err = s.ContactsBlockFromReplies(noticeCtx(3), &mtproto.TLContactsBlockFromReplies{MsgId: commentID}); status.Convert(err).Message() != "MSG_ID_INVALID" || blocked != 0 {
		t.Fatalf("channel id err %v blocked %d", err, blocked)
	}
	up, err := s.ContactsBlockFromReplies(noticeCtx(3), &mtproto.TLContactsBlockFromReplies{MsgId: 50})
	if err != nil {
		t.Fatal(err)
	}
	if blocked != 8 {
		t.Fatalf("blocked %d", blocked)
	}
	if up == nil || len(up.GetUpdates()) != 1 || up.GetUpdates()[0].GetPeerId().GetUserId() != 8 {
		t.Fatalf("updates %#v", up)
	}
	before := writes
	if _, err = svc.Send(ctx, 8, group.Id, 0, []channel.Outgoing{{
		RandomID: 4,
		Message: &mtproto.Message{
			Message: "after-block",
			Date:    8,
			ReplyTo: mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
				ReplyToMsgId:           parentID,
				ReplyToMsgId_INT32:     parentID,
				ReplyToMsgId_FLAGINT32: mtproto.MakeFlagsInt32(parentID),
			}).To_MessageReplyHeader(),
		},
	}}); err != nil || writes != before {
		t.Fatalf("after block writes %d %v", writes, err)
	}
}

type noticeInbox struct {
	rows map[int64]map[int32]*mtproto.MessageBox
}

func (n noticeInbox) MessageGetUserMessage(_ context.Context, in *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	row := n.rows[in.GetUserId()][in.GetId()]
	if row == nil {
		return nil, mtproto.ErrMsgIdInvalid
	}
	return row, nil
}

func (n noticeInbox) MessageGetHistoryMessages(context.Context, *message.TLMessageGetHistoryMessages) (*message.Vector_MessageBox, error) {
	return &message.Vector_MessageBox{}, nil
}

func noticeCtx(user int64) context.Context {
	out, err := rpcmd.RpcMetadataToOutgoing(context.Background(), &rpcmd.RpcMetadata{UserId: user, PermAuthKeyId: user + 10})
	if err != nil {
		panic(err)
	}
	md, _ := metadata.FromOutgoingContext(out)
	return metadata.NewIncomingContext(context.Background(), md)
}
