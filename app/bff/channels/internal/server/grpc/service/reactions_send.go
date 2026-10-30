package service

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	sync_client "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	chatpb "github.com/teamgram/teamgram-server/app/service/biz/chat/chat"
	chat_client "github.com/teamgram/teamgram-server/app/service/biz/chat/client"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/app/service/biz/reaction"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	idgen_client "github.com/teamgram/teamgram-server/app/service/idgen/client"

	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/logx"
)

// ReactionDeps is how messages.sendReaction reaches private chats, groups,
// and the other devices of the sender. Nil clients skip that step.
type ReactionDeps struct {
	Messages message_client.MessageClient
	Chats    *chat_client.ChatClientHelper
	Sync     sync_client.SyncClient
	IDGen    *idgen_client.IDGenClient2
	Users    user_client.UserClient
	// Box, BoxByDialog, and ChatUsers override the RPC clients in tests.
	Box         func(ctx context.Context, userID int64, msgID int32) (*mtproto.MessageBox, error)
	BoxByDialog func(ctx context.Context, userID, dialogMessageID int64) (*mtproto.MessageBox, error)
	ChatUsers   func(ctx context.Context, chatID int64) ([]int64, error)
	Push        func(ctx context.Context, userID int64, updates *mtproto.Updates)
}

func NewReactions(d ReactionDeps) *ReactionsService {
	return &ReactionsService{deps: reactionDeps{
		messages:    d.Messages,
		chats:       d.Chats,
		syncer:      d.Sync,
		idgen:       d.IDGen,
		users:       d.Users,
		box:         d.Box,
		boxByDialog: d.BoxByDialog,
		chatUsers:   d.ChatUsers,
		push:        d.Push,
	}}
}

type reactionDeps struct {
	messages    message_client.MessageClient
	chats       *chat_client.ChatClientHelper
	syncer      sync_client.SyncClient
	idgen       *idgen_client.IDGenClient2
	users       user_client.UserClient
	box         func(ctx context.Context, userID int64, msgID int32) (*mtproto.MessageBox, error)
	boxByDialog func(ctx context.Context, userID, dialogMessageID int64) (*mtproto.MessageBox, error)
	chatUsers   func(ctx context.Context, chatID int64) ([]int64, error)
	push        func(ctx context.Context, userID int64, updates *mtproto.Updates)
}

func (s ReactionsService) sendReaction(ctx context.Context, in *mtproto.TLMessagesSendReaction) (*mtproto.Updates, error) {
	self, auth := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	emos := reactionEmojis(in)
	switch peer.PeerType {
	case mtproto.PEER_CHANNEL:
		up, err := channel.Shared().React(ctx, self, peer.PeerId, in.GetMsgId(), emos, in.GetBig())
		if err != nil {
			return nil, err
		}
		if len(emos) > 0 {
			catalog.Must().NoteReaction(self, emos)
		}
		return up, nil
	case mtproto.PEER_USER, mtproto.PEER_SELF, mtproto.PEER_CHAT:
		return s.reactDialog(ctx, self, auth, peer, in.GetMsgId(), emos, in.GetBig())
	default:
		return nil, mtproto.ErrPeerIdInvalid
	}
}

func (s ReactionsService) reactDialog(ctx context.Context, self, auth int64, peer *mtproto.PeerUtil, msgID int32, emos []string, big bool) (*mtproto.Updates, error) {
	box, err := s.loadOwnBox(ctx, self, peer, msgID)
	if err != nil {
		return nil, err
	}
	if box == nil || box.DialogMessageId == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	owner := box.UserId
	if owner == 0 {
		owner = self
	}
	boxes := []reaction.Box{{
		UserID:   owner,
		PeerType: box.PeerType,
		PeerID:   box.PeerId,
		MsgID:    box.MessageId,
	}}
	others := s.otherBoxes(ctx, self, box)
	boxes = append(boxes, others...)
	key := reaction.Key{DialogID1: box.DialogId1, DialogID2: box.DialogId2, DialogMessageID: box.DialogMessageId}
	rows, err := reaction.Shared().Replace(ctx, key, self, emos, big, boxes)
	if err != nil {
		return nil, err
	}
	if len(emos) > 0 {
		catalog.Must().NoteReaction(self, emos)
	}
	users := s.reactionUsers(ctx, self, rows)
	resp := reaction.Updates(self, reaction.Peer(box.PeerType, box.PeerId), box.MessageId, rows, users, nil)
	s.notePts(ctx, self, reaction.Update(reaction.Peer(box.PeerType, box.PeerId), box.MessageId, reaction.Summary(rows, self)))
	s.pushNotMe(ctx, self, auth, resp)
	for _, b := range others {
		up := reaction.Updates(b.UserID, reaction.Peer(b.PeerType, b.PeerID), b.MsgID, rows, users, nil)
		s.notePts(ctx, b.UserID, reaction.Update(reaction.Peer(b.PeerType, b.PeerID), b.MsgID, reaction.Summary(rows, b.UserID)))
		s.pushUser(ctx, b.UserID, up)
	}
	return resp, nil
}

func (s ReactionsService) loadOwnBox(ctx context.Context, self int64, peer *mtproto.PeerUtil, msgID int32) (*mtproto.MessageBox, error) {
	box, err := s.userBox(ctx, self, msgID)
	if err != nil {
		return nil, err
	}
	if box != nil && boxMatches(box, peer, self) {
		return box, nil
	}
	// The id may be the other participant's box id (the sender's id, when the
	// two counters still match, is the common case and is handled above).
	if peer.PeerType == mtproto.PEER_USER && peer.PeerId != 0 && peer.PeerId != self {
		other, err := s.userBox(ctx, peer.PeerId, msgID)
		if err != nil {
			return nil, err
		}
		if other != nil && other.DialogMessageId != 0 {
			own, err := s.dialogBox(ctx, self, other.DialogMessageId)
			if err != nil {
				return nil, err
			}
			if own != nil && boxMatches(own, peer, self) {
				return own, nil
			}
		}
	}
	return nil, nil
}

func boxMatches(box *mtproto.MessageBox, peer *mtproto.PeerUtil, self int64) bool {
	switch peer.PeerType {
	case mtproto.PEER_SELF:
		return box.PeerType == mtproto.PEER_SELF || (box.PeerType == mtproto.PEER_USER && (box.PeerId == self || box.PeerId == 0))
	case mtproto.PEER_USER:
		return box.PeerType == mtproto.PEER_USER && box.PeerId == peer.PeerId
	case mtproto.PEER_CHAT:
		return box.PeerType == mtproto.PEER_CHAT && box.PeerId == peer.PeerId
	default:
		return false
	}
}

func (s ReactionsService) otherBoxes(ctx context.Context, self int64, box *mtproto.MessageBox) []reaction.Box {
	var ids []int64
	switch box.PeerType {
	case mtproto.PEER_USER:
		if box.PeerId != 0 && box.PeerId != self {
			ids = []int64{box.PeerId}
		}
	case mtproto.PEER_CHAT:
		ids = s.memberIDs(ctx, box.PeerId)
	}
	var out []reaction.Box
	for _, id := range ids {
		if id == 0 || id == self {
			continue
		}
		ob, err := s.dialogBox(ctx, id, box.DialogMessageId)
		if err != nil || ob == nil || ob.MessageId == 0 {
			continue
		}
		out = append(out, reaction.Box{
			UserID:   ob.UserId,
			PeerType: ob.PeerType,
			PeerID:   ob.PeerId,
			MsgID:    ob.MessageId,
		})
	}
	return out
}

func (s ReactionsService) userBox(ctx context.Context, userID int64, msgID int32) (*mtproto.MessageBox, error) {
	if s.deps.box != nil {
		return s.deps.box(ctx, userID, msgID)
	}
	if s.deps.messages == nil || msgID == 0 {
		return nil, nil
	}
	box, err := s.deps.messages.MessageGetUserMessage(ctx, &message.TLMessageGetUserMessage{UserId: userID, Id: msgID})
	if err != nil {
		logx.WithContext(ctx).Errorf("reaction box %d/%d: %v", userID, msgID, err)
		return nil, nil
	}
	return box, nil
}

func (s ReactionsService) dialogBox(ctx context.Context, userID, dialogMessageID int64) (*mtproto.MessageBox, error) {
	if s.deps.boxByDialog != nil {
		return s.deps.boxByDialog(ctx, userID, dialogMessageID)
	}
	if s.deps.messages == nil || dialogMessageID == 0 {
		return nil, nil
	}
	list, err := s.deps.messages.MessageGetUserMessageListByDataIdList(ctx, &message.TLMessageGetUserMessageListByDataIdList{
		UserId: userID,
		IdList: []int64{dialogMessageID},
	})
	if err != nil || list == nil {
		return nil, err
	}
	for _, b := range list.GetDatas() {
		if b != nil && b.DialogMessageId == dialogMessageID {
			return b, nil
		}
	}
	return nil, nil
}

func (s ReactionsService) memberIDs(ctx context.Context, chatID int64) []int64 {
	if s.deps.chatUsers != nil {
		ids, _ := s.deps.chatUsers(ctx, chatID)
		return ids
	}
	if s.deps.chats == nil {
		return nil
	}
	chat, err := s.deps.chats.Client().ChatGetMutableChat(ctx, &chatpb.TLChatGetMutableChat{ChatId: chatID})
	if err != nil || chat == nil {
		return nil
	}
	return chat.ParticipantIdList()
}

func (s ReactionsService) reactionUsers(ctx context.Context, self int64, rows []reaction.Row) []*mtproto.User {
	if s.deps.users == nil || len(rows) == 0 {
		return reaction.Users(rows)
	}
	var ids []int64
	seen := map[int64]bool{}
	for _, r := range rows {
		if r.UserID != 0 && !seen[r.UserID] {
			seen[r.UserID] = true
			ids = append(ids, r.UserID)
		}
	}
	users, err := s.deps.users.UserGetMutableUsers(ctx, &user.TLUserGetMutableUsers{Id: ids, To: []int64{self}})
	if err != nil || users == nil {
		return reaction.Users(rows)
	}
	out := users.GetUserListByIdList(self, ids...)
	if len(out) == 0 {
		return reaction.Users(rows)
	}
	return out
}

func (s ReactionsService) notePts(ctx context.Context, userID int64, upd *mtproto.Update) {
	if s.deps.idgen == nil || upd == nil || userID == 0 {
		return
	}
	pts := s.deps.idgen.NextPtsId(ctx, userID)
	if pts == 0 {
		return
	}
	raw, err := jsonx.Marshal(upd)
	if err != nil {
		return
	}
	if err = reaction.Shared().SavePts(ctx, userID, pts, 1, mtproto.GetUpdateType(upd), string(raw), time.Now().Unix()); err != nil {
		logx.WithContext(ctx).Errorf("reaction pts %d: %v", userID, err)
	}
}

func (s ReactionsService) pushUser(ctx context.Context, userID int64, up *mtproto.Updates) {
	if s.deps.push != nil {
		s.deps.push(ctx, userID, up)
		return
	}
	if s.deps.syncer == nil || up == nil {
		return
	}
	_, _ = s.deps.syncer.SyncPushUpdates(ctx, &sync.TLSyncPushUpdates{UserId: userID, Updates: up})
}

func (s ReactionsService) pushNotMe(ctx context.Context, userID, auth int64, up *mtproto.Updates) {
	if s.deps.push != nil || s.deps.syncer == nil || auth == 0 || up == nil {
		return
	}
	_, _ = s.deps.syncer.SyncUpdatesNotMe(ctx, &sync.TLSyncUpdatesNotMe{
		UserId:        userID,
		PermAuthKeyId: auth,
		Updates:       up,
	})
}

func (s ReactionsService) getMessagesReactions(ctx context.Context, in *mtproto.TLMessagesGetMessagesReactions) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer.PeerType == mtproto.PEER_CHANNEL {
		return channel.Shared().ReactionsFor(ctx, self, peer.PeerId, in.GetId())
	}
	var ups []*mtproto.Update
	for _, id := range in.GetId() {
		box, err := s.loadOwnBox(ctx, self, peer, id)
		if err != nil {
			return nil, err
		}
		if box == nil {
			ups = append(ups, reaction.Update(reaction.Peer(peer.PeerType, peer.PeerId), id, reaction.Summary(nil, self)))
			continue
		}
		rows, err := reaction.Shared().List(ctx, reaction.Key{box.DialogId1, box.DialogId2, box.DialogMessageId})
		if err != nil {
			return nil, err
		}
		ups = append(ups, reaction.Update(reaction.Peer(box.PeerType, box.PeerId), box.MessageId, reaction.Summary(rows, self)))
	}
	if ups == nil {
		ups = []*mtproto.Update{}
	}
	return mtproto.MakeUpdatesByUpdates(ups...), nil
}

func (s ReactionsService) getMessageReactionsList(ctx context.Context, in *mtproto.TLMessagesGetMessageReactionsList) (*mtproto.Messages_MessageReactionsList, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	var rows []reaction.Row
	var err error
	if peer.PeerType == mtproto.PEER_CHANNEL {
		rows, err = channel.Shared().ReactionRows(ctx, peer.PeerId, in.GetId())
	} else {
		box, loadErr := s.loadOwnBox(ctx, self, peer, in.GetId())
		if loadErr != nil {
			return nil, loadErr
		}
		if box != nil {
			rows, err = reaction.Shared().List(ctx, reaction.Key{box.DialogId1, box.DialogId2, box.DialogMessageId})
		}
	}
	if err != nil {
		return nil, err
	}
	emo := ""
	if in.GetReaction_FLAGREACTION() != nil {
		emo = in.GetReaction_FLAGREACTION().GetEmoticon()
	}
	if emo == "" && in.GetReaction_FLAGSTRING() != nil {
		emo = in.GetReaction_FLAGSTRING().GetValue()
	}
	page, total := reaction.ListPage(rows, emo, in.GetOffset().GetValue(), in.GetLimit())
	var peerRows []*mtproto.MessagePeerReaction
	for _, r := range page {
		peerRows = append(peerRows, mtproto.MakeTLMessagePeerReaction(&mtproto.MessagePeerReaction{
			Big:               r.Big,
			My:                r.UserID == self,
			PeerId:            mtproto.MakePeerUser(r.UserID),
			Date:              r.Date,
			Reaction_REACTION: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: r.Emoticon}).To_Reaction(),
			Reaction_STRING:   r.Emoticon,
		}).To_MessagePeerReaction())
	}
	if peerRows == nil {
		peerRows = []*mtproto.MessagePeerReaction{}
	}
	return mtproto.MakeTLMessagesMessageReactionsList(&mtproto.Messages_MessageReactionsList{
		Count:     total,
		Reactions: peerRows,
		Chats:     []*mtproto.Chat{},
		Users:     s.reactionUsers(ctx, self, page),
	}).To_Messages_MessageReactionsList(), nil
}

func reactionEmojis(in *mtproto.TLMessagesSendReaction) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range in.GetReaction_FLAGVECTORREACTION() {
		emo := ""
		if r != nil {
			emo = r.GetEmoticon()
		}
		if emo == "" || seen[emo] {
			continue
		}
		seen[emo] = true
		out = append(out, emo)
	}
	if len(out) == 0 && in.GetReaction_FLAGSTRING() != nil {
		if emo := in.GetReaction_FLAGSTRING().GetValue(); emo != "" {
			out = append(out, emo)
		}
	}
	return out
}
