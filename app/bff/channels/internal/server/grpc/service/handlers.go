package service

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
)

func (s *Service) md(ctx context.Context) *metadata.RpcMetadata {
	return metadata.RpcMetadataFromIncoming(ctx)
}

func (s *Service) ChannelsCreateChannel(ctx context.Context, in *mtproto.TLChannelsCreateChannel) (*mtproto.Updates, error) {
	md := s.md(ctx)
	up, err := channel.Shared().Create(ctx, channel.CreateRequest{
		CreatorID: md.GetUserId(),
		ActorAuth: md.GetPermAuthKeyId(),
		Title:     in.GetTitle(),
		About:     in.GetAbout(),
		Broadcast: in.GetBroadcast(),
		Megagroup: in.GetMegagroup(),
		Forum:     in.GetForum(),
		TtlPeriod: in.GetTtlPeriod().GetValue(),
		Address:   in.GetAddress().GetValue(),
	})
	if err != nil {
		return nil, err
	}
	up.Users = s.users(ctx, md.GetUserId(), md.GetUserId())
	return up, nil
}

func (s *Service) ChannelsGetChannels(ctx context.Context, in *mtproto.TLChannelsGetChannels) (*mtproto.Messages_Chats, error) {
	return channel.Shared().GetChannels(ctx, s.md(ctx).GetUserId(), in.GetId())
}

func (s *Service) ChannelsGetFullChannel(ctx context.Context, in *mtproto.TLChannelsGetFullChannel) (*mtproto.Messages_ChatFull, error) {
	md := s.md(ctx)
	full, err := channel.Shared().GetFullChannel(ctx, md.GetUserId(), in.GetChannel())
	if err != nil {
		return nil, err
	}
	full.Users = s.users(ctx, md.GetUserId(), md.GetUserId())
	return full, nil
}

func (s *Service) ChannelsGetParticipants(ctx context.Context, in *mtproto.TLChannelsGetParticipants) (*mtproto.Channels_ChannelParticipants, error) {
	md := s.md(ctx)
	res, err := channel.Shared().GetParticipants(ctx, md.GetUserId(), in.GetChannel(), in.GetFilter(), in.GetOffset(), in.GetLimit(), in.GetHash())
	if err != nil || res.Participants == nil {
		return res, err
	}
	ids := make([]int64, 0, len(res.Participants))
	for _, p := range res.Participants {
		if p.GetUserId() != 0 {
			ids = append(ids, p.GetUserId())
		} else if p.GetPeer() != nil {
			ids = append(ids, p.GetPeer().GetUserId())
		}
	}
	res.Users = s.users(ctx, md.GetUserId(), ids...)
	return res, nil
}

func (s *Service) ChannelsGetParticipant(ctx context.Context, in *mtproto.TLChannelsGetParticipant) (*mtproto.Channels_ChannelParticipant, error) {
	md := s.md(ctx)
	res, err := channel.Shared().GetParticipant(ctx, md.GetUserId(), in.GetChannel(), in.GetParticipant())
	if err != nil {
		return nil, err
	}
	id := res.GetParticipant().GetUserId()
	if id == 0 && res.GetParticipant().GetPeer() != nil {
		id = res.GetParticipant().GetPeer().GetUserId()
	}
	res.Users = s.users(ctx, md.GetUserId(), id)
	return res, nil
}

func (s *Service) ChannelsInviteToChannelC9E33D54(ctx context.Context, in *mtproto.TLChannelsInviteToChannelC9E33D54) (*mtproto.Messages_InvitedUsers, error) {
	return s.invite(ctx, in.GetChannel(), in.GetUsers())
}

func (s *Service) ChannelsInviteToChannel199F3A6C(ctx context.Context, in *mtproto.TLChannelsInviteToChannel199F3A6C) (*mtproto.Updates, error) {
	invited, err := s.invite(ctx, in.GetChannel(), in.GetUsers())
	if err != nil {
		return nil, err
	}
	return invited.GetUpdates(), nil
}

func (s *Service) invite(ctx context.Context, ch *mtproto.InputChannel, users []*mtproto.InputUser) (*mtproto.Messages_InvitedUsers, error) {
	md := s.md(ctx)
	ids := make([]int64, 0, len(users))
	bots := map[int64]bool{}
	for _, u := range users {
		if u == nil {
			continue
		}
		switch u.PredicateName {
		case mtproto.Predicate_inputUser:
			ids = append(ids, u.UserId)
		case mtproto.Predicate_inputUserSelf:
			ids = append(ids, md.GetUserId())
		}
	}
	return channel.Shared().Invite(ctx, md.GetUserId(), md.GetPermAuthKeyId(), ch, ids, bots)
}

func (s *Service) ChannelsJoinChannel24B524C5(ctx context.Context, in *mtproto.TLChannelsJoinChannel24B524C5) (*mtproto.Updates, error) {
	md := s.md(ctx)
	return channel.Shared().Join(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel())
}

func (s *Service) ChannelsJoinChannel7F6A1E22(ctx context.Context, in *mtproto.TLChannelsJoinChannel7F6A1E22) (*mtproto.Messages_ChatInviteJoinResult, error) {
	md := s.md(ctx)
	up, err := channel.Shared().Join(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesChatInviteJoinResultOk(&mtproto.Messages_ChatInviteJoinResult{
		Updates: up,
	}).To_Messages_ChatInviteJoinResult(), nil
}

func (s *Service) ChannelsLeaveChannel(ctx context.Context, in *mtproto.TLChannelsLeaveChannel) (*mtproto.Updates, error) {
	md := s.md(ctx)
	return channel.Shared().Leave(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel())
}

func (s *Service) ChannelsEditAdmin(ctx context.Context, in *mtproto.TLChannelsEditAdmin) (*mtproto.Updates, error) {
	md := s.md(ctx)
	userID := in.GetUserId().GetUserId()
	if in.GetUserId().GetPredicateName() == mtproto.Predicate_inputUserSelf {
		userID = md.GetUserId()
	}
	rank := in.GetRank_STRING()
	if in.GetRank_FLAGSTRING() != nil {
		rank = in.GetRank_FLAGSTRING().GetValue()
	}
	return channel.Shared().EditAdmin(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), userID, in.GetAdminRights(), rank)
}

func (s *Service) ChannelsEditBanned(ctx context.Context, in *mtproto.TLChannelsEditBanned) (*mtproto.Updates, error) {
	md := s.md(ctx)
	peer := mtproto.FromInputPeer2(md.GetUserId(), in.GetParticipant())
	return channel.Shared().EditBanned(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), peer.PeerId, in.GetBannedRights())
}

func (s *Service) ChannelsEditTitle(ctx context.Context, in *mtproto.TLChannelsEditTitle) (*mtproto.Updates, error) {
	md := s.md(ctx)
	return channel.Shared().EditTitle(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), in.GetTitle())
}

func (s *Service) ChannelsEditPhoto(ctx context.Context, in *mtproto.TLChannelsEditPhoto) (*mtproto.Updates, error) {
	md := s.md(ctx)
	return channel.Shared().EditPhoto(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), in.GetPhoto())
}

func (s *Service) ChannelsToggleSignatures(ctx context.Context, in *mtproto.TLChannelsToggleSignatures) (*mtproto.Updates, error) {
	md := s.md(ctx)
	return channel.Shared().ToggleSignatures(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), mtproto.FromBool(in.GetEnabled()))
}

func (s *Service) ChannelsGetAdminedPublicChannels(ctx context.Context, in *mtproto.TLChannelsGetAdminedPublicChannels) (*mtproto.Messages_Chats, error) {
	return channel.Shared().AdminedPublicChannels(ctx, s.md(ctx).GetUserId())
}

func (s *Service) ChannelsDeleteChannel(ctx context.Context, in *mtproto.TLChannelsDeleteChannel) (*mtproto.Updates, error) {
	md := s.md(ctx)
	return channel.Shared().DeleteChannel(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel())
}

func (s *Service) ChannelsExportMessageLink(ctx context.Context, in *mtproto.TLChannelsExportMessageLink) (*mtproto.ExportedMessageLink, error) {
	return channel.Shared().ExportMessageLink(ctx, s.md(ctx).GetUserId(), in.GetChannel(), in.GetId())
}

func (s *Service) ChannelsReadHistory(ctx context.Context, in *mtproto.TLChannelsReadHistory) (*mtproto.Bool, error) {
	if err := channel.Shared().ReadHistory(ctx, s.md(ctx).GetUserId(), in.GetChannel(), in.GetMaxId()); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func (s *Service) ChannelsGetMessages(ctx context.Context, in *mtproto.TLChannelsGetMessages) (*mtproto.Messages_Messages, error) {
	md := s.md(ctx)
	var ids []int32
	for _, m := range in.GetId_VECTORINPUTMESSAGE() {
		if m == nil {
			continue
		}
		switch m.PredicateName {
		case mtproto.Predicate_inputMessageID, mtproto.Predicate_inputMessageReplyTo:
			ids = append(ids, m.GetId())
		}
	}
	ids = append(ids, in.GetId_VECTORINT32()...)
	res, err := channel.Shared().GetMessages(ctx, md.GetUserId(), in.GetChannel(), ids)
	if err != nil {
		return nil, err
	}
	var userIDs []int64
	for _, m := range res.Messages {
		if m.GetFromId().GetUserId() != 0 {
			userIDs = append(userIDs, m.GetFromId().GetUserId())
		}
	}
	res.Users = s.users(ctx, md.GetUserId(), userIDs...)
	return res, nil
}

func (s *Service) ChannelsDeleteMessages(ctx context.Context, in *mtproto.TLChannelsDeleteMessages) (*mtproto.Messages_AffectedMessages, error) {
	md := s.md(ctx)
	pts, count, err := channel.Shared().DeleteInput(ctx, md.GetUserId(), md.GetPermAuthKeyId(), in.GetChannel(), in.GetId())
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessagesAffectedMessages(&mtproto.Messages_AffectedMessages{
		Pts:      pts,
		PtsCount: count,
	}).To_Messages_AffectedMessages(), nil
}
