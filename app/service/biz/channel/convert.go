package channel

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/pkg/brand"
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
	feat := s.features(ctx, ch.ID)
	now := int32(time.Now().Unix())
	var admin *mtproto.ChatAdminRights
	var banned *mtproto.ChatBannedRights
	if p != nil && p.IsAdmin() {
		admin = p.adminRights().ToChatAdminRights()
	}
	if p != nil && p.Role == RoleBanned {
		banned = mtproto.IssuedBannedRights(p.BannedRights, now)
	}
	chat := mtproto.MakeTLChannel(&mtproto.Chat{
		Creator:                     p.IsCreator(),
		Left:                        !member,
		Broadcast:                   ch.Broadcast,
		Megagroup:                   ch.Megagroup,
		Forum:                       ch.Forum,
		Signatures:                  ch.Signatures,
		SignatureProfiles:           feat.SignatureProfiles,
		Id:                          ch.ID,
		AccessHash_FLAGINT64:        &wrapperspb.Int64Value{Value: ch.AccessHash},
		Title:                       ch.Title,
		Username:                    optionalString(ch.Username),
		Photo:                       chatPhoto(ch.PhotoID),
		Date:                        ch.Date,
		AdminRights:                 admin,
		BannedRights:                banned,
		DefaultBannedRights:         mtproto.IssuedBannedRights(ch.DefaultBanned, now),
		ParticipantsCount_FLAGINT32: &wrapperspb.Int32Value{Value: ch.ParticipantsCount},
		HasGeo:                      ch.Address != "" || feat.Lat != 0 || feat.Long != 0,
		Noforwards:                  feat.NoForwards,
		SlowmodeEnabled:             feat.SlowmodeSeconds > 0,
		JoinRequest:                 feat.JoinRequest,
		Gigagroup:                   feat.Gigagroup,
		Autotranslation:             feat.Autotranslation,
		BroadcastMessagesAllowed:    feat.BroadcastMessagesAllowed,
		Usernames:                   usernameVector(listedUsernames(ch, feat)),
		Color_FLAGPEERCOLOR:         peerColorValue(feat.ColorSet, feat.ColorID, feat.ColorEmoji),
		ProfileColor:                peerColorValue(feat.ProfileColorSet, feat.ProfileColorID, feat.ProfileColorEmoji),
		EmojiStatus:                 emojiStatusValue(feat.Emoji),
		SendPaidMessagesStars:       paidStars(feat),
	}).To_Chat()
	if LookupGroupCall != nil && ch != nil {
		if att := LookupGroupCall(ctx, ch.ID); att.Active && att.Call != nil {
			chat.CallActive = true
			chat.CallNotEmpty = att.NotEmpty
		}
	}
	if LookupStories != nil && ch != nil {
		att := LookupStories(ctx, viewer, ch.ID)
		if att.MaxID > 0 {
			chat.StoriesUnavailable = false
			chat.StoriesMaxId_FLAGRECENTSTORY = mtproto.MakeTLRecentStory(&mtproto.RecentStory{
				Live:  att.Live,
				MaxId: &wrapperspb.Int32Value{Value: att.MaxID},
			}).To_RecentStory()
		} else if !chat.Min {
			chat.StoriesUnavailable = true
		}
		chat.StoriesHidden = att.Hidden
	}
	return chat
}

// leftChannelView is the channel card for channels.getLeftChannels.
// chatFor hides a private channel once the viewer is no longer a member,
// which made every private megagroup the user left look like an empty list.
// The left list still shows the title, with left set, including a private one.
func (s *Service) leftChannelView(ctx context.Context, viewer int64, ch *Channel, p *Participant) *mtproto.Chat {
	if ch == nil || ch.Deleted {
		return mtproto.MakeTLChannelForbidden(&mtproto.Chat{}).To_Chat()
	}
	if p == nil && s != nil && s.store != nil {
		p, _ = s.store.GetParticipant(ctx, ch.ID, viewer)
	}
	view := p
	if p != nil && !p.InChannel() {
		clone := *p
		clone.State = StateNormal
		clone.Role = RoleMember
		clone.AdminRights = 0
		clone.BannedRights = 0
		view = &clone
	}
	chat := s.chatFor(ctx, viewer, ch, view)
	if chat.GetPredicateName() == mtproto.Predicate_channel {
		chat.Left = true
		chat.Creator = false
		chat.AdminRights = nil
	}
	return chat
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
		CanViewParticipants:  member || ch.Public(),
		CanSetUsername:       p.IsCreator(),
		CanSetStickers:       p.IsCreator(),
		CanViewStats:         p.IsCreator(),
		CanDeleteChannel:     p.IsCreator(),
		TranslationsDisabled: chatbox.Shared().TranslationsDisabled(viewer, mtproto.MakePeerUtil(mtproto.PEER_CHANNEL, ch.ID)),
		Id:                   ch.ID,
		About:                ch.About,
		ParticipantsCount:    &wrapperspb.Int32Value{Value: ch.ParticipantsCount},
		ReadInboxMaxId:       st.ReadInboxMaxID,
		ReadOutboxMaxId:      st.ReadOutboxMaxID,
		UnreadCount:          st.UnreadCount,
		ChatPhoto:            mtproto.MakeTLPhotoEmpty(nil).To_Photo(),
		NotifySettings:       defaultNotify(),
		BotInfo:              []*mtproto.BotInfo{},
		Pts:                  ch.Pts,
	}).To_ChatFull()
	if ch.MigratedFromChatID != 0 {
		full.MigratedFromChatId = &wrapperspb.Int64Value{Value: ch.MigratedFromChatID}
		full.MigratedFromMaxId = &wrapperspb.Int32Value{Value: ch.MigratedFromMaxID}
	}
	if ch.TtlPeriod != 0 {
		full.TtlPeriod = &wrapperspb.Int32Value{Value: ch.TtlPeriod}
	}
	if feat := s.features(ctx, ch.ID); feat != nil {
		full.RestrictedSponsored = feat.SponsoredRestricted
		full.HiddenPrehistory = feat.HiddenPrehistory
		full.ParticipantsHidden = feat.ParticipantsHidden
		full.Antispam = feat.Antispam
		full.ViewForumAsMessages = feat.ViewForumAsMessages[viewer]
		full.MainTab = mtproto.ToProfileTabByType(feat.MainProfileTab)
		if feat.SlowmodeSeconds > 0 {
			full.SlowmodeSeconds = &wrapperspb.Int32Value{Value: feat.SlowmodeSeconds}
		}
		if feat.PinnedMsgID > 0 {
			full.PinnedMsgId = &wrapperspb.Int32Value{Value: feat.PinnedMsgID}
		}
		if feat.LinkedChatID != 0 {
			full.LinkedChatId = &wrapperspb.Int64Value{Value: feat.LinkedChatID}
		}
		if feat.AvailableMinID > 0 {
			full.AvailableMinId = &wrapperspb.Int32Value{Value: feat.AvailableMinID}
		}
		if feat.BoostsUnrestrict > 0 {
			full.BoostsUnrestrict = &wrapperspb.Int32Value{Value: feat.BoostsUnrestrict}
		}
		if stars := paidStars(feat); stars != nil {
			full.SendPaidMessagesStars = stars
		}
		if reactions := chatReactionsOf(feat); reactions != nil {
			full.AvailableReactions_FLAGCHATREACTIONS = reactions
		}
		if feat.ReactionLimit > 0 {
			full.ReactionsLimit = &wrapperspb.Int32Value{Value: feat.ReactionLimit}
		}
		full.PaidReactionsAvailable = feat.PaidReactions
		if feat.Lat != 0 || feat.Long != 0 || ch.Address != "" {
			full.Location = mtproto.MakeTLChannelLocation(&mtproto.ChannelLocation{
				GeoPoint: mtproto.MakeTLGeoPoint(&mtproto.GeoPoint{Lat: feat.Lat, Long: feat.Long}).To_GeoPoint(),
				Address:  ch.Address,
			}).To_ChannelLocation()
		}
	}
	if admins, kicked, banned, err := s.participantCounts(ctx, ch.ID); err == nil {
		full.AdminsCount = &wrapperspb.Int32Value{Value: admins}
		full.KickedCount = &wrapperspb.Int32Value{Value: kicked}
		full.BannedCount = &wrapperspb.Int32Value{Value: banned}
	}
	if LookupGroupCall != nil {
		if att := LookupGroupCall(ctx, ch.ID); att.Call != nil {
			full.Call = att.Call
		}
	}
	if LookupStories != nil {
		att := LookupStories(ctx, viewer, ch.ID)
		full.Stories = att.Stories
		full.StoriesPinnedAvailable = att.Pinned
	}
	// The invite sheet reads this before it calls messages.exportChatInvite.
	// Without it the iOS sheet stays on the progress bar.
	if p != nil && p.InChannel() && (p.IsCreator() || p.adminRights().CanInviteUsers()) {
		if inv := s.PrimaryExported(ctx, ch.ID, viewer); inv != nil {
			full.ExportedInvite = inv
		}
		if pending, recent := s.PendingRequesters(ctx, ch.ID); pending > 0 {
			full.RequestsPending = &wrapperspb.Int32Value{Value: pending}
			full.RecentRequesters = recent
		}
	}
	return full
}

func participantTL(actor int64, p *Participant) *mtproto.ChannelParticipant {
	return participantFor(actor, nil, nil, p)
}

// participantFor builds one ChannelParticipant for viewer.
// channelParticipantAdmin shares flags.1 between self and inviter_id.
// When the row is the viewer, inviter_id has to be on the wire or the
// client reads promoted_by as inviter_id and the next constructor is garbage.
// https://core.telegram.org/constructor/channelParticipantAdmin
func participantFor(actor int64, viewer *Participant, promoted map[int64]int64, p *Participant) *mtproto.ChannelParticipant {
	rank := optionalString(p.Rank)
	now := int32(time.Now().Unix())
	liveBan := mtproto.IssuedBannedRights(p.BannedRights, now)
	switch {
	case p.Role == RoleCreator:
		return mtproto.MakeTLChannelParticipantCreator(&mtproto.ChannelParticipant{
			UserId:      p.UserID,
			AdminRights: mtproto.AdminRights(allAdminRights()).ToChatAdminRights(),
			Rank:        rank,
		}).To_ChannelParticipant()
	case p.Role == RoleAdmin && p.State == StateNormal:
		self := actor != 0 && p.UserID == actor
		var inviter *wrapperspb.Int64Value
		if self {
			inviter = &wrapperspb.Int64Value{Value: p.InviterID}
		}
		canEdit := true
		if viewer != nil {
			canEdit = viewerMayEditAdmin(viewer, promoted, p)
		}
		return mtproto.MakeTLChannelParticipantAdmin(&mtproto.ChannelParticipant{
			UserId:              p.UserID,
			InviterId_FLAGINT64: inviter,
			PromotedBy:          promoterOf(promoted, p),
			Date:                p.Date,
			AdminRights:         adminRightsTL(p.AdminRights),
			Rank:                rank,
			CanEdit:             canEdit,
			Self:                self,
		}).To_ChannelParticipant()
	case p.State == StateKicked || (p.Role == RoleBanned && liveBan != nil):
		rights := liveBan
		if p.State == StateKicked && rights == nil {
			rights = mtproto.BannedRights(p.BannedRights).ToChatBannedRights()
		}
		return mtproto.MakeTLChannelParticipantBanned(&mtproto.ChannelParticipant{
			Peer:         mtproto.MakePeerUser(p.UserID),
			KickedBy:     p.InviterID,
			Date:         p.Date,
			BannedRights: rights,
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

func adminRightsTL(bits int32) *mtproto.ChatAdminRights {
	if rights := mtproto.AdminRights(bits).ToChatAdminRights(); rights != nil {
		return rights
	}
	return mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{}).To_ChatAdminRights()
}

func promoterOf(promoted map[int64]int64, p *Participant) int64 {
	if p == nil {
		return 0
	}
	if promoted != nil {
		if id := promoted[p.UserID]; id != 0 {
			return id
		}
	}
	return p.InviterID
}

func rememberPromoter(f *Features, userID, by int64) {
	if f == nil {
		return
	}
	if f.PromotedBy == nil {
		f.PromotedBy = map[int64]int64{}
	}
	if by == 0 {
		delete(f.PromotedBy, userID)
		return
	}
	f.PromotedBy[userID] = by
}

// viewerMayEditAdmin is the channels.editAdmin rule the client shows as can_edit.
// The creator can edit every other admin. An admin can edit only admins they
// promoted, and only while they still have add_admins. Nobody edits themselves.
func viewerMayEditAdmin(viewer *Participant, promoted map[int64]int64, target *Participant) bool {
	if viewer == nil || target == nil || target.UserID == viewer.UserID {
		return false
	}
	if viewer.IsCreator() {
		return true
	}
	if !viewer.IsAdmin() || !viewer.adminRights().CanAddAdmins() {
		return false
	}
	return promoterOf(promoted, target) == viewer.UserID
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
	// Old rows may already contain a zero-length hashtag or a duplicated
	// URL. Drop them before the message is returned to a client.
	msg.SanitizeEntities()
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
	return brand.LinkBase()
}
