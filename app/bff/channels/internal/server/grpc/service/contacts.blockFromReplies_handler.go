package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/channels/repliesdelete"
	"github.com/teamgram/teamgram-server/app/bff/userchannelprofiles/statusbook"
	"github.com/teamgram/teamgram-server/app/messenger/msg/msg/msg"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/logx"
)

// repliesUserID is the @replies service user. Telegram clients address
// discussion-reply notifications to this account (DrKLO/Telegram
// MessagesController.repliesUserId).
const repliesUserID int64 = 1271266957

type repliesUsers interface {
	UserGetMutableUsers(ctx context.Context, in *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error)
	UserBlockPeer(ctx context.Context, in *user.TLUserBlockPeer) (*mtproto.Bool, error)
}

type repliesMessages interface {
	MessageGetUserMessage(ctx context.Context, in *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error)
	MessageGetHistoryMessages(ctx context.Context, in *message.TLMessageGetHistoryMessages) (*message.Vector_MessageBox, error)
}

type repliesOutbox interface {
	MsgDeleteMessages(ctx context.Context, in *msg.TLMsgDeleteMessages) (*mtproto.Messages_AffectedMessages, error)
}

type repliesSync interface {
	SyncUpdatesNotMe(ctx context.Context, in *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error)
}

// ContactsBlockFromReplies
// contacts.blockFromReplies#29a8962c flags:# delete_message:flags.0?true delete_history:flags.1?true report_spam:flags.2?true msg_id:int = Updates;
//
// Stops notifications about discussion replies from the sender of a
// message in @replies. https://core.telegram.org/method/contacts.blockFromReplies
// 400 MSG_ID_INVALID — msg_id is not a message in that chat.
func (s ThreadsService) ContactsBlockFromReplies(ctx context.Context, in *mtproto.TLContactsBlockFromReplies) (*mtproto.Updates, error) {
	self, authKey := actor(ctx)
	if self == 0 || in.GetMsgId() <= 0 || s.Messages == nil {
		return nil, mtproto.ErrMsgIdInvalid
	}
	box, err := s.Messages.MessageGetUserMessage(ctx, &message.TLMessageGetUserMessage{
		UserId: self,
		Id:     in.GetMsgId(),
	})
	if err != nil || box == nil || !isRepliesBox(box) {
		return nil, mtproto.ErrMsgIdInvalid
	}
	sender := replySender(box)
	if sender == 0 || sender == self || sender == repliesUserID {
		return nil, mtproto.ErrMsgIdInvalid
	}
	if s.Users != nil {
		if _, err := s.Users.UserBlockPeer(ctx, &user.TLUserBlockPeer{
			UserId:   self,
			PeerType: mtproto.PEER_USER,
			PeerId:   sender,
		}); err != nil {
			return nil, err
		}
		if err := statusbook.Shared().Forget(ctx, self, sender); err != nil {
			return nil, err
		}
	}
	if in.GetReportSpam() {
		logx.WithContext(ctx).Infof("contacts.blockFromReplies report_spam user %d msg %d", sender, in.GetMsgId())
	}
	if (in.GetDeleteMessage() || in.GetDeleteHistory()) && s.Outbox != nil {
		if err := s.deleteReplies(ctx, self, authKey, in.GetMsgId(), in.GetDeleteHistory()); err != nil {
			return nil, err
		}
	}
	up := s.blockedUpdates(ctx, self, sender)
	if s.Sync != nil {
		_, _ = s.Sync.SyncUpdatesNotMe(ctx, &sync.TLSyncUpdatesNotMe{
			UserId:        self,
			PermAuthKeyId: authKey,
			Updates:       up,
		})
	}
	return up, nil
}

func (s ThreadsService) deleteReplies(ctx context.Context, self, authKey int64, msgID int32, history bool) error {
	del := func(ctx context.Context, ids []int32) error {
		if len(ids) == 0 || s.Outbox == nil {
			return nil
		}
		_, err := s.Outbox.MsgDeleteMessages(ctx, &msg.TLMsgDeleteMessages{
			UserId:    self,
			AuthKeyId: authKey,
			PeerType:  mtproto.PEER_EMPTY,
			PeerId:    0,
			Revoke:    false,
			Id:        ids,
		})
		return err
	}
	if !history || s.Messages == nil {
		return del(ctx, []int32{msgID})
	}
	// DeleteHistory reads msg_id again and leaves that row in place until
	// the walk finishes. SelectByMessageId only returns deleted = 0, so
	// deleting it first would make a retry MSG_ID_INVALID.
	return repliesdelete.DeleteHistory(ctx,
		func(ctx context.Context, userID int64, id int32) (*mtproto.MessageBox, error) {
			return s.Messages.MessageGetUserMessage(ctx, &message.TLMessageGetUserMessage{UserId: userID, Id: id})
		},
		func(ctx context.Context, offset int32) ([]*mtproto.MessageBox, error) {
			hist, err := s.Messages.MessageGetHistoryMessages(ctx, &message.TLMessageGetHistoryMessages{
				UserId:     self,
				PeerType:   mtproto.PEER_USER,
				PeerId:     repliesUserID,
				OffsetId:   offset,
				OffsetDate: repliesdelete.RawOffsetDate,
				Limit:      repliesdelete.Page,
			})
			if err != nil || hist == nil {
				return nil, err
			}
			return hist.GetDatas(), nil
		}, del, self, msgID)
}

func (s ThreadsService) blockedUpdates(ctx context.Context, self, sender int64) *mtproto.Updates {
	peer := mtproto.MakeTLPeerUser(&mtproto.Peer{UserId: sender}).To_Peer()
	upd := mtproto.MakeTLUpdatePeerBlocked(&mtproto.Update{
		Blocked_BOOL:         mtproto.BoolTrue,
		Blocked_FLAGBOOLEAN:  true,
		BlockedMyStoriesFrom: false,
		PeerId:               peer,
	}).To_Update()
	var users []*mtproto.User
	if s.Users != nil {
		loaded, err := s.Users.UserGetMutableUsers(ctx, &user.TLUserGetMutableUsers{
			Id: []int64{self, sender},
			To: []int64{self},
		})
		if err == nil && loaded != nil {
			users = loaded.GetUserListByIdList(self, self, sender)
		}
	}
	return mtproto.MakeUpdatesByUpdatesUsers(users, upd)
}

func isRepliesBox(box *mtproto.MessageBox) bool {
	return box.GetPeerType() == mtproto.PEER_USER && box.GetPeerId() == repliesUserID
}

func replySender(box *mtproto.MessageBox) int64 {
	return repliesdelete.AuthorOf(box)
}

func uniqueIDs(ids []int32) []int32 {
	seen := map[int32]bool{}
	out := make([]int32, 0, len(ids))
	for _, id := range ids {
		if id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, id)
	}
	return out
}
