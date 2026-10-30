package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
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
	}
	if in.GetReportSpam() {
		logx.WithContext(ctx).Infof("contacts.blockFromReplies report_spam user %d msg %d", sender, in.GetMsgId())
	}
	if (in.GetDeleteMessage() || in.GetDeleteHistory()) && s.Outbox != nil {
		ids := []int32{in.GetMsgId()}
		if in.GetDeleteHistory() {
			ids = append(ids, s.repliesFromSender(ctx, self, sender)...)
		}
		ids = uniqueIDs(ids)
		_, _ = s.Outbox.MsgDeleteMessages(ctx, &msg.TLMsgDeleteMessages{
			UserId:    self,
			AuthKeyId: authKey,
			PeerType:  mtproto.PEER_USER,
			PeerId:    repliesUserID,
			Revoke:    false,
			Id:        ids,
		})
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

func (s ThreadsService) repliesFromSender(ctx context.Context, self, sender int64) []int32 {
	var ids []int32
	var offset int32
	for page := 0; page < 5; page++ {
		hist, err := s.Messages.MessageGetHistoryMessages(ctx, &message.TLMessageGetHistoryMessages{
			UserId:   self,
			PeerType: mtproto.PEER_USER,
			PeerId:   repliesUserID,
			OffsetId: offset,
			Limit:    100,
		})
		if err != nil || hist == nil || len(hist.GetDatas()) == 0 {
			break
		}
		var oldest int32
		for _, box := range hist.GetDatas() {
			if box == nil {
				continue
			}
			if replySender(box) == sender {
				ids = append(ids, box.GetMessageId())
			}
			if oldest == 0 || (box.GetMessageId() != 0 && box.GetMessageId() < oldest) {
				oldest = box.GetMessageId()
			}
		}
		if len(hist.GetDatas()) < 100 || oldest == 0 || oldest == offset {
			break
		}
		offset = oldest
	}
	return ids
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
	if box == nil || box.GetMessage() == nil || box.GetMessage().GetFwdFrom() == nil {
		if box != nil && box.GetSenderUserId() != 0 && box.GetSenderUserId() != repliesUserID {
			return box.GetSenderUserId()
		}
		return 0
	}
	fwd := box.GetMessage().GetFwdFrom()
	if id := userPeerID(fwd.GetFromId()); id != 0 {
		return id
	}
	if id := userPeerID(fwd.GetSavedFromId()); id != 0 {
		return id
	}
	if box.GetSenderUserId() != 0 && box.GetSenderUserId() != repliesUserID {
		return box.GetSenderUserId()
	}
	return 0
}

func userPeerID(p *mtproto.Peer) int64 {
	if p == nil || p.GetPredicateName() != mtproto.Predicate_peerUser {
		return 0
	}
	return p.GetUserId()
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
