package channel

import (
	"context"
	"os"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (s *Service) ChatFor(ctx context.Context, viewer int64, ch *Channel) *mtproto.Chat {
	return s.chatFor(ctx, viewer, ch, nil)
}

func (s *Service) chatFor(ctx context.Context, viewer int64, ch *Channel, p *Participant) *mtproto.Chat {
	if ch == nil || ch.Deleted {
		return mtproto.MakeTLChannelForbidden(&mtproto.Chat{}).To_Chat()
	}
	if p == nil && s != nil && s.store != nil {
		p, _ = s.store.GetParticipant(ctx, ch.ID, viewer)
	}
	member := p != nil && p.InChannel()
	if !member && !ch.Public() {
		return forbiddenChat(ch)
	}
	var admin *mtproto.ChatAdminRights
	var banned *mtproto.ChatBannedRights
	if p != nil && p.IsAdmin() {
		admin = p.adminRights().ToChatAdminRights()
	}
	if p != nil && p.Role == RoleBanned && p.BannedRights != 0 {
		banned = mtproto.BannedRights(p.BannedRights).ToChatBannedRights()
	}
	return mtproto.MakeTLChannel(&mtproto.Chat{
		Creator:                     p.IsCreator(),
		Left:                        !member,
		Broadcast:                   ch.Broadcast,
		Megagroup:                   ch.Megagroup,
		Forum:                       ch.Forum,
		Signatures:                  ch.Signatures,
		Id:                          ch.ID,
		AccessHash_FLAGINT64:        &wrapperspb.Int64Value{Value: ch.AccessHash},
		Title:                       ch.Title,
		Username:                    optionalString(ch.Username),
		Photo:                       chatPhoto(ch.PhotoID),
		Date:                        ch.Date,
		AdminRights:                 admin,
		BannedRights:                banned,
		DefaultBannedRights:         mtproto.BannedRights(ch.DefaultBanned).ToChatBannedRights(),
		ParticipantsCount_FLAGINT32: &wrapperspb.Int32Value{Value: ch.ParticipantsCount},
		HasGeo:                      ch.Address != "",
	}).To_Chat()
}

func forbiddenChat(ch *Channel) *mtproto.Chat {
	return mtproto.MakeTLChannelForbidden(&mtproto.Chat{
		Broadcast:        ch.Broadcast,
		Megagroup:        ch.Megagroup,
		Id:               ch.ID,
		AccessHash_INT64: ch.AccessHash,
		Title:            ch.Title,
	}).To_Chat()
}

func chatPhoto(photoID int64) *mtproto.ChatPhoto {
	if photoID == 0 {
		return mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto()
	}
	return mtproto.MakeTLChatPhoto(&mtproto.ChatPhoto{
		PhotoId: photoID,
		DcId:    1,
	}).To_ChatPhoto()
}

func optionalString(s string) *wrapperspb.StringValue {
	if s == "" {
		return nil
	}
	return &wrapperspb.StringValue{Value: s}
}

func defaultNotify() *mtproto.PeerNotifySettings {
	return mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
		ShowPreviews: mtproto.BoolTrue,
		Silent:       mtproto.BoolFalse,
	}).To_PeerNotifySettings()
}

func (s *Service) fullChat(ctx context.Context, viewer int64, ch *Channel, p *Participant) *mtproto.ChatFull {
	st, _ := s.store.GetRead(ctx, ch.ID, viewer)
	if st == nil {
		st = &ReadState{}
	}
	member := p != nil && p.InChannel()
	full := mtproto.MakeTLChannelFull(&mtproto.ChatFull{
		CanViewParticipants: member || ch.Public(),
		CanSetUsername:      p.IsCreator(),
		CanSetStickers:      p.IsCreator(),
		CanViewStats:        p.IsCreator(),
		CanDeleteChannel:    p.IsCreator(),
		Id:                  ch.ID,
		About:               ch.About,
		ParticipantsCount:   &wrapperspb.Int32Value{Value: ch.ParticipantsCount},
		ReadInboxMaxId:      st.ReadInboxMaxID,
		ReadOutboxMaxId:     st.ReadOutboxMaxID,
		UnreadCount:         st.UnreadCount,
		ChatPhoto:           mtproto.MakeTLPhotoEmpty(nil).To_Photo(),
		NotifySettings:      defaultNotify(),
		BotInfo:             []*mtproto.BotInfo{},
		Pts:                 ch.Pts,
	}).To_ChatFull()
	if ch.MigratedFromChatID != 0 {
		full.MigratedFromChatId = &wrapperspb.Int64Value{Value: ch.MigratedFromChatID}
		full.MigratedFromMaxId = &wrapperspb.Int32Value{Value: ch.MigratedFromMaxID}
	}
	if ch.TtlPeriod != 0 {
		full.TtlPeriod = &wrapperspb.Int32Value{Value: ch.TtlPeriod}
	}
	if admins, kicked, banned, err := s.participantCounts(ctx, ch.ID); err == nil {
		full.AdminsCount = &wrapperspb.Int32Value{Value: admins}
		full.KickedCount = &wrapperspb.Int32Value{Value: kicked}
		full.BannedCount = &wrapperspb.Int32Value{Value: banned}
	}
	return full
}

func participantTL(actor int64, p *Participant) *mtproto.ChannelParticipant {
	rank := optionalString(p.Rank)
	switch {
	case p.Role == RoleCreator:
		return mtproto.MakeTLChannelParticipantCreator(&mtproto.ChannelParticipant{
			UserId:      p.UserID,
			AdminRights: mtproto.AdminRights(allAdminRights()).ToChatAdminRights(),
			Rank:        rank,
		}).To_ChannelParticipant()
	case p.Role == RoleAdmin && p.State == StateNormal:
		return mtproto.MakeTLChannelParticipantAdmin(&mtproto.ChannelParticipant{
			UserId:          p.UserID,
			InviterId_INT64: p.InviterID,
			PromotedBy:      p.InviterID,
			Date:            p.Date,
			AdminRights:     mtproto.AdminRights(p.AdminRights).ToChatAdminRights(),
			Rank:            rank,
			CanEdit:         true,
			Self:            p.UserID == actor,
		}).To_ChannelParticipant()
	case p.State == StateKicked || (p.Role == RoleBanned && p.BannedRights != 0):
		return mtproto.MakeTLChannelParticipantBanned(&mtproto.ChannelParticipant{
			Peer:         mtproto.MakePeerUser(p.UserID),
			KickedBy:     p.InviterID,
			Date:         p.Date,
			BannedRights: mtproto.BannedRights(p.BannedRights).ToChatBannedRights(),
			Left:         p.State != StateNormal,
		}).To_ChannelParticipant()
	case p.State == StateLeft:
		return mtproto.MakeTLChannelParticipantLeft(&mtproto.ChannelParticipant{
			Peer: mtproto.MakePeerUser(p.UserID),
		}).To_ChannelParticipant()
	case p.UserID == actor:
		return mtproto.MakeTLChannelParticipantSelf(&mtproto.ChannelParticipant{
			UserId:          p.UserID,
			InviterId_INT64: p.InviterID,
			Date:            p.Date,
		}).To_ChannelParticipant()
	default:
		return mtproto.MakeTLChannelParticipant(&mtproto.ChannelParticipant{
			UserId: p.UserID,
			Date:   p.Date,
		}).To_ChannelParticipant()
	}
}

func viewMessage(m *mtproto.Message, senderID, viewer int64) *mtproto.Message {
	if m == nil {
		return nil
	}
	out := proto.Clone(m).(*mtproto.Message)
	out.Out = senderID == viewer
	return out
}

func unmarshalMessage(data []byte) (*mtproto.Message, error) {
	msg := &mtproto.Message{}
	if err := proto.Unmarshal(data, msg); err != nil {
		return nil, err
	}
	return msg, nil
}

func newMessageUpdate(msg *mtproto.Message, pts, count int32) *mtproto.Update {
	return mtproto.MakeTLUpdateNewChannelMessage(&mtproto.Update{
		Message_MESSAGE: msg,
		Pts_INT32:       pts,
		PtsCount:        count,
	}).To_Update()
}

func editMessageUpdate(msg *mtproto.Message, pts, count int32) *mtproto.Update {
	return mtproto.MakeTLUpdateEditChannelMessage(&mtproto.Update{
		Message_MESSAGE: msg,
		Pts_INT32:       pts,
		PtsCount:        count,
	}).To_Update()
}

func deleteUpdate(channelID int64, ids []int32, pts, count int32) *mtproto.Update {
	return mtproto.MakeTLUpdateDeleteChannelMessages(&mtproto.Update{
		ChannelId: channelID,
		Messages:  ids,
		Pts_INT32: pts,
		PtsCount:  count,
	}).To_Update()
}

func channelUpdate(channelID int64) *mtproto.Update {
	return mtproto.MakeTLUpdateChannel(&mtproto.Update{ChannelId: channelID}).To_Update()
}

func linkHost() string {
	if h := strings.TrimRight(os.Getenv("TEAMGRAM_TME_HOST"), "/"); h != "" {
		return h
	}
	return "https://t.me"
}
