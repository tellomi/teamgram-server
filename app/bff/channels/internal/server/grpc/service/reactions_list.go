package service

import (
	"context"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/accesshash"
)

// reactionUserAPI loads the users named by a reaction list.
type reactionUserAPI interface {
	UserGetMutableUsers(ctx context.Context, in *user.TLUserGetMutableUsers) (*user.Vector_ImmutableUser, error)
}

// reactionMessageAPI loads a private or basic-group message by id.
type reactionMessageAPI interface {
	MessageGetUserMessage(ctx context.Context, in *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error)
}

// messageReactionsList is messages.getMessageReactionsList.
//
// https://core.telegram.org/method/messages.getMessageReactionsList
// https://core.telegram.org/api/reactions
func (s ReactionsService) messageReactionsList(ctx context.Context, in *mtproto.TLMessagesGetMessageReactionsList) (*mtproto.Messages_MessageReactionsList, error) {
	self, err := reactionUser(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := reactionPeer(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	reaction := in.GetReaction_FLAGREACTION()
	if reaction == nil {
		if legacy := in.GetReaction_FLAGSTRING(); legacy != nil && legacy.GetValue() != "" {
			reaction = mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: legacy.GetValue()}).To_Reaction()
		}
	}
	offset := ""
	if in.GetOffset() != nil {
		offset = in.GetOffset().GetValue()
	}
	var list *mtproto.Messages_MessageReactionsList
	if peer.IsChannel() {
		list, err = channel.Shared().ReactionList(ctx, self, peer.PeerId, peer.AccessHash, in.GetId(), reaction, offset, in.GetLimit())
	} else {
		if err = s.requirePrivateMessage(ctx, self, peer, in.GetId()); err != nil {
			return nil, err
		}
		list, err = chatbox.Shared().ReactionList(ctx, self, peer, in.GetId(), reaction, offset, in.GetLimit())
	}
	if err != nil {
		return nil, err
	}
	list.Users = s.attachUsers(ctx, self, reactionUserIDs(list.GetReactions()))
	if list.Chats == nil {
		list.Chats = []*mtproto.Chat{}
	}
	if list.Reactions == nil {
		list.Reactions = []*mtproto.MessagePeerReaction{}
	}
	return list, nil
}

// messagesReactions is messages.getMessagesReactions.
//
// https://core.telegram.org/method/messages.getMessagesReactions
func (s ReactionsService) messagesReactions(ctx context.Context, in *mtproto.TLMessagesGetMessagesReactions) (*mtproto.Updates, error) {
	self, err := reactionUser(ctx)
	if err != nil {
		return nil, err
	}
	peer, err := reactionPeer(self, in.GetPeer())
	if err != nil {
		return nil, err
	}
	var up *mtproto.Updates
	if peer.IsChannel() {
		up, err = channel.Shared().ReactionsFor(ctx, self, peer.PeerId, in.GetId())
	} else {
		ids, keepErr := s.keepPrivateMessages(ctx, self, peer, in.GetId())
		if keepErr != nil {
			return nil, keepErr
		}
		up, err = chatbox.Shared().ReactionsFor(ctx, self, peer, ids)
	}
	if err != nil {
		return nil, err
	}
	return s.withReactionUsers(ctx, self, up), nil
}

func reactionUser(ctx context.Context) (int64, error) {
	md := metadata.RpcMetadataFromIncoming(ctx)
	if md != nil && md.GetIsBot() {
		return 0, mtproto.ErrBotMethodInvalid
	}
	if md == nil {
		return 0, nil
	}
	return md.GetUserId(), nil
}

func reactionPeer(self int64, in *mtproto.InputPeer) (*mtproto.PeerUtil, error) {
	if in == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	peer := mtproto.FromInputPeer2(self, in)
	if peer == nil || !peer.IsUserOrChatOrChannel() || (peer.PeerId == 0 && !peer.IsSelf()) {
		return nil, mtproto.ErrPeerIdInvalid
	}
	return peer, nil
}

func (s ReactionsService) requirePrivateMessage(ctx context.Context, self int64, peer *mtproto.PeerUtil, id int32) error {
	if id <= 0 {
		return mtproto.ErrMsgIdInvalid
	}
	if s.Messages == nil {
		return nil
	}
	box, err := s.Messages.MessageGetUserMessage(ctx, &message.TLMessageGetUserMessage{UserId: self, Id: id})
	if missingMessage(err) {
		return mtproto.ErrMsgIdInvalid
	}
	if err != nil {
		return err
	}
	if box == nil || box.GetMessageId() == 0 || !boxPeerMatches(box, peer, self) {
		return mtproto.ErrMsgIdInvalid
	}
	return nil
}

func (s ReactionsService) keepPrivateMessages(ctx context.Context, self int64, peer *mtproto.PeerUtil, ids []int32) ([]int32, error) {
	if len(ids) == 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	var keep []int32
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if s.Messages == nil {
			keep = append(keep, id)
			continue
		}
		box, err := s.Messages.MessageGetUserMessage(ctx, &message.TLMessageGetUserMessage{UserId: self, Id: id})
		if missingMessage(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if box == nil || box.GetMessageId() == 0 || !boxPeerMatches(box, peer, self) {
			continue
		}
		keep = append(keep, id)
	}
	if len(keep) == 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	return keep, nil
}

func missingMessage(err error) bool {
	if err == nil {
		return false
	}
	if err == mtproto.ErrMessageIdInvalid || err == mtproto.ErrMsgIdInvalid {
		return true
	}
	// The message service is reached over gRPC, so the not-found error is a status string.
	msg := err.Error()
	return strings.Contains(msg, "MESSAGE_ID_INVALID") || strings.Contains(msg, "MSG_ID_INVALID")
}

func boxPeerMatches(box *mtproto.MessageBox, peer *mtproto.PeerUtil, self int64) bool {
	wantType := peer.PeerType
	wantID := peer.PeerId
	if peer.PeerType == mtproto.PEER_SELF {
		wantType = mtproto.PEER_USER
		wantID = self
		if box.GetPeerType() == mtproto.PEER_SELF && (box.GetPeerId() == self || box.GetPeerId() == 0) {
			return true
		}
	}
	if wantType == mtproto.PEER_USER {
		// A private message row carries the dialog as the sorted pair of user
		// ids. The recipient's copy stores peer_id = recipient (itself), so
		// peer_id alone does not name the chat for incoming messages.
		if a, b := box.GetDialogId1(), box.GetDialogId2(); a != 0 || b != 0 {
			x, y := self, wantID
			if x > y {
				x, y = y, x
			}
			return a == x && b == y
		}
		if box.GetPeerType() == mtproto.PEER_USER && box.GetPeerId() == self && box.GetSenderUserId() == wantID {
			return true
		}
	}
	return box.GetPeerType() == wantType && box.GetPeerId() == wantID
}

func (s ReactionsService) attachUsers(ctx context.Context, self int64, ids []int64) []*mtproto.User {
	ids = uniqIDs(ids)
	if s.Users != nil && len(ids) > 0 {
		query := append([]int64{self}, ids...)
		got, err := s.Users.UserGetMutableUsers(ctx, &user.TLUserGetMutableUsers{
			Id: query,
			To: []int64{self},
		})
		if err == nil && got != nil {
			if list := got.GetUserListByIdList(self, ids...); len(list) > 0 {
				return list
			}
		}
	}
	return stubReactionUsers(ctx, self, ids)
}

func (s ReactionsService) withReactionUsers(ctx context.Context, self int64, up *mtproto.Updates) *mtproto.Updates {
	if up == nil {
		return up
	}
	var ids []int64
	for _, u := range up.Updates {
		ids = append(ids, reactionUserIDs(u.GetReactions_MESSAGEREACTIONS().GetRecentReactions())...)
	}
	if len(ids) == 0 {
		if up.Users == nil {
			up.Users = []*mtproto.User{}
		}
		return up
	}
	up.Users = s.attachUsers(ctx, self, ids)
	return up
}

func reactionUserIDs(rows []*mtproto.MessagePeerReaction) []int64 {
	var ids []int64
	for _, row := range rows {
		if id := row.GetPeerId().GetUserId(); id != 0 {
			ids = append(ids, id)
		}
	}
	return ids
}

func uniqIDs(ids []int64) []int64 {
	var out []int64
	seen := map[int64]struct{}{}
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func stubReactionUsers(ctx context.Context, self int64, ids []int64) []*mtproto.User {
	var out []*mtproto.User
	for _, id := range ids {
		out = append(out, mtproto.MakeTLUser(&mtproto.User{Id: id, Self: id == self}).To_User())
	}
	if out == nil {
		out = []*mtproto.User{}
	}
	if !accesshash.Issuing() {
		return out
	}
	accesshash.PaintUsers(ctx, out)
	for i, u := range out {
		if u.GetAccessHash().GetValue() == 0 {
			out[i] = mtproto.MakeTLUserEmpty(&mtproto.User{Id: u.GetId()}).To_User()
		}
	}
	return out
}
