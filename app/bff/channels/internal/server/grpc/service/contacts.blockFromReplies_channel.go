package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"google.golang.org/grpc/status"
)

// replyBlockSender finds who contacts.blockFromReplies should block.
// A message in the @replies chat wins: that is the id official clients send.
// Any other result, including a private message that merely shares the id,
// is looked up as a supergroup reply or a discussion comment.
// TELLOMI_CHANNEL_COMMENTS is not required for that lookup and is not changed.
func (s ThreadsService) replyBlockSender(ctx context.Context, self int64, msgID int32) (sender int64, fromReplies bool, err error) {
	if s.Messages != nil {
		box, lookupErr := s.Messages.MessageGetUserMessage(ctx, &message.TLMessageGetUserMessage{
			UserId: self,
			Id:     msgID,
		})
		if lookupErr != nil && !replyMessageMissing(lookupErr) {
			return 0, false, mtproto.ErrMsgIdInvalid
		}
		if lookupErr == nil && box != nil && isRepliesBox(box) {
			return replySender(box), true, nil
		}
		// A private message with the same id is a different chat. Channel
		// message ids are not stored in that table, and a low id such as 3
		// or 5 is usually already a private message. Keep looking.
	}
	sender, err = channel.Shared().ReplyAuthorForBlock(ctx, self, msgID)
	return sender, false, err
}

func replyMessageMissing(err error) bool {
	switch status.Convert(err).Message() {
	case "MESSAGE_ID_INVALID", "MSG_ID_INVALID":
		return true
	default:
		return false
	}
}
