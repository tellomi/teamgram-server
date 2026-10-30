package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

// MessagesGetUnreadReactions returns messages that still have reactions the
// caller has not opened.
//
// https://core.telegram.org/method/messages.getUnreadReactions
func (ReactionsService) MessagesGetUnreadReactions(ctx context.Context, in *mtproto.TLMessagesGetUnreadReactions) (*mtproto.Messages_Messages, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	// Saved-message slices are a separate dialog. This batch does not index
	// reactions by saved_peer_id, so the slice is empty rather than mixing
	// the parent chat's reactions into it.
	if in.GetSavedPeerId() != nil {
		return chatbox.PageMessages(peer, 0, nil, nil), nil
	}
	if peer.IsChannel() {
		return channel.Shared().UnreadReactions(ctx, self, peer.PeerId, flag32(in.GetTopMsgId()), in.GetOffsetId(), in.GetAddOffset(), in.GetLimit(), in.GetMaxId(), in.GetMinId())
	}
	msgs, err := chatbox.Shared().UnreadReactions(ctx, self, peer, in.GetOffsetId(), in.GetAddOffset(), in.GetLimit(), in.GetMaxId(), in.GetMinId())
	if err != nil {
		return nil, err
	}
	return chatbox.PageMessages(peer, 0, msgs, nil), nil
}

// MessagesReadReactions marks reactions in the dialog as read.
//
// https://core.telegram.org/method/messages.readReactions
func (ReactionsService) MessagesReadReactions(ctx context.Context, in *mtproto.TLMessagesReadReactions) (*mtproto.Messages_AffectedHistory, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		return channel.Shared().ReadReactions(ctx, self, peer.PeerId, flag32(in.GetTopMsgId()))
	}
	if err := chatbox.Shared().ReadReactions(ctx, self, peer); err != nil {
		return nil, err
	}
	return chatbox.AffectedHistory(0, 0), nil
}

// MessagesSetChatAvailableReactions chooses which reactions a chat accepts.
//
// https://core.telegram.org/method/messages.setChatAvailableReactions
func (ReactionsService) MessagesSetChatAvailableReactions(ctx context.Context, in *mtproto.TLMessagesSetChatAvailableReactions) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	mode, allow, emos, docs, err := channel.ParseChatReactions(in.GetAvailableReactions_CHATREACTIONS(), in.GetAvailableReactions_VECTORSTRING())
	if err != nil {
		return nil, err
	}
	var limit int32
	if in.GetReactionsLimit() != nil {
		limit = in.GetReactionsLimit().GetValue()
		if limit < 1 || limit > 11 {
			return nil, mtproto.ErrReactionsTooMany
		}
	}
	var paid *bool
	if in.GetPaidEnabled() != nil {
		v := mtproto.FromBool(in.GetPaidEnabled())
		paid = &v
	}
	if peer.IsChannel() {
		return channel.Shared().SetAvailableReactions(ctx, self, peer.PeerId, mode, allow, emos, docs, limit, paid)
	}
	if err := chatbox.Shared().SetAvailableReactions(ctx, peer, mode, allow, emos, docs, limit, paid); err != nil {
		return nil, err
	}
	return mtproto.MakeEmptyUpdates(), nil
}

// MessagesSetDefaultReaction sets the account double-tap reaction.
//
// https://core.telegram.org/method/messages.setDefaultReaction
func (ReactionsService) MessagesSetDefaultReaction(ctx context.Context, in *mtproto.TLMessagesSetDefaultReaction) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	reaction := in.GetReaction_REACTION()
	if reaction == nil && in.GetReaction_STRING() != "" {
		reaction = mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: in.GetReaction_STRING()}).To_Reaction()
	}
	if err := chatbox.Shared().SetDefaultReaction(ctx, self, reaction); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// MessagesDeleteParticipantReaction removes one member's reaction on a message.
//
// https://core.telegram.org/method/messages.deleteParticipantReaction
func (ReactionsService) MessagesDeleteParticipantReaction(ctx context.Context, in *mtproto.TLMessagesDeleteParticipantReaction) (*mtproto.Updates, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	who := mtproto.FromInputPeer2(self, in.GetParticipant())
	if peer == nil || !peer.IsUserOrChatOrChannel() || who == nil || !who.IsUser() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		return channel.Shared().DeleteParticipantReaction(ctx, self, peer.PeerId, in.GetMsgId(), who.PeerId)
	}
	return chatbox.Shared().DeleteParticipantReaction(ctx, self, peer, in.GetMsgId(), who.PeerId, false)
}

// MessagesDeleteParticipantReactions removes every reaction by one member.
//
// https://core.telegram.org/method/messages.deleteParticipantReactions
func (ReactionsService) MessagesDeleteParticipantReactions(ctx context.Context, in *mtproto.TLMessagesDeleteParticipantReactions) (*mtproto.Bool, error) {
	self, _ := actor(ctx)
	peer := mtproto.FromInputPeer2(self, in.GetPeer())
	who := mtproto.FromInputPeer2(self, in.GetParticipant())
	if peer == nil || !peer.IsUserOrChatOrChannel() || who == nil || !who.IsUser() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	if peer.IsChannel() {
		return channel.Shared().DeleteParticipantReactions(ctx, self, peer.PeerId, who.PeerId)
	}
	if err := chatbox.Shared().DeleteParticipantReactions(ctx, self, peer, who.PeerId, false); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func flag32(v interface{ GetValue() int32 }) int32 {
	if v == nil {
		return 0
	}
	return v.GetValue()
}
