package channel

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/brand"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (s *Service) features(ctx context.Context, channelID int64) *Features {
	f, err := s.store.GetFeatures(ctx, channelID)
	if err != nil || f == nil {
		return &Features{}
	}
	if f.LastSend == nil {
		f.LastSend = map[int64]int32{}
	}
	if f.UserMinID == nil {
		f.UserMinID = map[int64]int32{}
	}
	if f.ReadContents == nil {
		f.ReadContents = map[int64][]int32{}
	}
	if f.MsgTopic == nil {
		f.MsgTopic = map[int32]int32{}
	}
	if f.ReactionRead == nil {
		f.ReactionRead = map[int64]int32{}
	}
	if f.Views == nil {
		f.Views = map[int32]ViewRec{}
	}
	if f.InboxReadAt == nil {
		f.InboxReadAt = map[int64]int32{}
	}
	return f
}

func (s *Service) saveFeatures(ctx context.Context, channelID int64, f *Features) error {
	return s.store.SaveFeatures(ctx, channelID, f)
}

func (s *Service) logAdmin(ctx context.Context, channelID, userID int64, action *mtproto.ChannelAdminLogEventAction) {
	f := s.features(ctx, channelID)
	raw, err := proto.Marshal(action)
	if err != nil {
		return
	}
	f.LogSeq++
	f.AdminLog = append(f.AdminLog, AdminEvent{
		ID:     f.LogSeq,
		Date:   int32(time.Now().Unix()),
		UserID: userID,
		Action: raw,
	})
	_ = s.saveFeatures(ctx, channelID, f)
}

func (s *Service) GetAdminLog(ctx context.Context, self int64, in *mtproto.InputChannel, q string, filter *mtproto.ChannelAdminLogEventsFilter, admins []int64, maxID, minID int64, limit int32) (*mtproto.Channels_AdminLogResults, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	f := s.features(ctx, ch.ID)
	adminSet := map[int64]bool{}
	for _, id := range admins {
		adminSet[id] = true
	}
	q = strings.ToLower(q)
	var events []*mtproto.ChannelAdminLogEvent
	for i := len(f.AdminLog) - 1; i >= 0; i-- {
		ev := f.AdminLog[i]
		if maxID != 0 && ev.ID >= maxID {
			continue
		}
		if minID != 0 && ev.ID <= minID {
			continue
		}
		if len(adminSet) > 0 && !adminSet[ev.UserID] {
			continue
		}
		action := &mtproto.ChannelAdminLogEventAction{}
		if err := proto.Unmarshal(ev.Action, action); err != nil {
			continue
		}
		if !adminLogVisible(filter, action) {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(action.String()), q) {
			continue
		}
		events = append(events, mtproto.MakeTLChannelAdminLogEvent(&mtproto.ChannelAdminLogEvent{
			Id:     ev.ID,
			Date:   ev.Date,
			UserId: ev.UserID,
			Action: action,
		}).To_ChannelAdminLogEvent())
		if int32(len(events)) >= limit {
			break
		}
	}
	if events == nil {
		events = []*mtproto.ChannelAdminLogEvent{}
	}
	return mtproto.MakeTLChannelsAdminLogResults(&mtproto.Channels_AdminLogResults{
		Events: events,
		Chats:  []*mtproto.Chat{s.ChatFor(ctx, self, ch)},
		Users:  []*mtproto.User{},
	}).To_Channels_AdminLogResults(), nil
}

func adminLogVisible(filter *mtproto.ChannelAdminLogEventsFilter, action *mtproto.ChannelAdminLogEventAction) bool {
	if filter == nil {
		return true
	}
	name := action.GetPredicateName()
	switch name {
	case mtproto.Predicate_channelAdminLogEventActionParticipantJoin, mtproto.Predicate_channelAdminLogEventActionParticipantJoinByInvite:
		return filter.Join
	case mtproto.Predicate_channelAdminLogEventActionParticipantLeave:
		return filter.Leave
	case mtproto.Predicate_channelAdminLogEventActionParticipantInvite:
		return filter.Invite
	case mtproto.Predicate_channelAdminLogEventActionParticipantToggleBan:
		return filter.Ban || filter.Kick || filter.Unban || filter.Unkick
	case mtproto.Predicate_channelAdminLogEventActionParticipantToggleAdmin:
		return filter.Promote || filter.Demote
	case mtproto.Predicate_channelAdminLogEventActionChangeTitle, mtproto.Predicate_channelAdminLogEventActionChangeAbout, mtproto.Predicate_channelAdminLogEventActionChangeUsername, mtproto.Predicate_channelAdminLogEventActionChangePhoto:
		return filter.Info
	case mtproto.Predicate_channelAdminLogEventActionToggleSlowMode, mtproto.Predicate_channelAdminLogEventActionTogglePreHistoryHidden, mtproto.Predicate_channelAdminLogEventActionChangeLocation:
		return filter.Settings
	case mtproto.Predicate_channelAdminLogEventActionUpdatePinned:
		return filter.Pinned
	case mtproto.Predicate_channelAdminLogEventActionEditMessage:
		return filter.Edit
	case mtproto.Predicate_channelAdminLogEventActionDeleteMessage:
		return filter.Delete
	case mtproto.Predicate_channelAdminLogEventActionCreateTopic, mtproto.Predicate_channelAdminLogEventActionEditTopic, mtproto.Predicate_channelAdminLogEventActionDeleteTopic:
		return filter.Forums
	default:
		return filter.Settings || filter.Info
	}
}

func (s *Service) SetStickers(ctx context.Context, self int64, in *mtproto.InputChannel, set *mtproto.InputStickerSet) (*mtproto.Bool, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsCreator() && !p.adminRights().CanChangeInfo() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	if set == nil || set.GetPredicateName() == mtproto.Predicate_inputStickerSetEmpty {
		f.StickerSetID = 0
	} else {
		f.StickerSetID = set.GetId()
	}
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, self, mtproto.MakeTLChannelAdminLogEventActionChangeStickerSet(&mtproto.ChannelAdminLogEventAction{
		PrevStickerset: mtproto.MakeTLInputStickerSetEmpty(&mtproto.InputStickerSet{}).To_InputStickerSet(),
		NewStickerset:  set,
	}).To_ChannelAdminLogEventAction())
	return mtproto.BoolTrue, nil
}

func (s *Service) SetEmojiStickers(ctx context.Context, self int64, in *mtproto.InputChannel, set *mtproto.InputStickerSet) (*mtproto.Bool, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	if set == nil || set.GetPredicateName() == mtproto.Predicate_inputStickerSetEmpty {
		f.EmojiSetID = 0
	} else {
		f.EmojiSetID = set.GetId()
	}
	return mtproto.BoolTrue, s.saveFeatures(ctx, ch.ID, f)
}

func (s *Service) ReadContents(ctx context.Context, self int64, in *mtproto.InputChannel, ids []int32) error {
	ch, _, err := s.requireMember(ctx, self, in)
	if err != nil {
		return err
	}
	f := s.features(ctx, ch.ID)
	seen := map[int32]bool{}
	for _, id := range f.ReadContents[self] {
		seen[id] = true
	}
	for _, id := range ids {
		if !seen[id] {
			f.ReadContents[self] = append(f.ReadContents[self], id)
		}
	}
	return s.saveFeatures(ctx, ch.ID, f)
}

// DeleteByDate removes channel messages whose date is inside [minDate, maxDate].
// A zero bound is open. The caller must be allowed to delete messages.
func (s *Service) DeleteByDate(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, minDate, maxDate int32) (pts, ptsCount int32, err error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return 0, 0, err
	}
	if !p.IsCreator() && !p.adminRights().CanDeleteMessages() {
		return 0, 0, mtproto.ErrChatAdminRequired
	}
	msgs, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return 0, 0, err
	}
	var ids []int32
	for _, m := range msgs {
		if m == nil || m.Deleted {
			continue
		}
		if minDate > 0 && m.Date < minDate {
			continue
		}
		if maxDate > 0 && m.Date > maxDate {
			continue
		}
		ids = append(ids, m.ID)
	}
	if len(ids) == 0 {
		return ch.Pts, 0, nil
	}
	return s.Delete(ctx, self, ch.ID, actorAuth, ids)
}

func (s *Service) DeleteHistory(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, maxID int32, forEveryone bool) (pts, ptsCount int32, err error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return 0, 0, err
	}
	if maxID <= 0 || maxID > ch.TopMessage {
		maxID = ch.TopMessage
	}
	f := s.features(ctx, ch.ID)
	if !forEveryone {
		if f.UserMinID[self] < maxID {
			f.UserMinID[self] = maxID
		}
		return ch.Pts, 0, s.saveFeatures(ctx, ch.ID, f)
	}
	if !p.IsAdmin() || (!p.IsCreator() && !p.adminRights().CanDeleteMessages()) {
		return 0, 0, mtproto.ErrChatAdminRequired
	}
	msgs, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return 0, 0, err
	}
	var ids []int32
	for _, m := range msgs {
		if !m.Deleted && m.ID <= maxID {
			ids = append(ids, m.ID)
		}
	}
	pts, ptsCount, _, err = s.store.CommitDelete(ctx, ch.ID, ids)
	if err != nil {
		return 0, 0, err
	}
	f = s.features(ctx, ch.ID)
	if f.AvailableMinID < maxID {
		f.AvailableMinID = maxID
	}
	if err = s.saveFeatures(ctx, ch.ID, f); err != nil {
		return 0, 0, err
	}
	if ptsCount > 0 {
		s.emit(ctx, self, actorAuth, ch.ID, func(viewer int64) *mtproto.Updates {
			return s.updatesWith(ctx, viewer, ch, deleteUpdate(ch.ID, ids, pts, ptsCount))
		})
	}
	return pts, ptsCount, nil
}

func (s *Service) DeleteParticipantHistory(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, userID int64) (pts, ptsCount, offset int32, err error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return 0, 0, 0, err
	}
	if userID != self && !p.IsCreator() && !p.adminRights().CanDeleteMessages() {
		return 0, 0, 0, mtproto.ErrChatAdminRequired
	}
	msgs, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return 0, 0, 0, err
	}
	var ids []int32
	for _, m := range msgs {
		if !m.Deleted && m.SenderID == userID {
			ids = append(ids, m.ID)
		}
	}
	pts, ptsCount, err = s.Delete(ctx, self, ch.ID, actorAuth, ids)
	return pts, ptsCount, 0, err
}

func (s *Service) TogglePreHistory(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, enabled bool) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !ch.Megagroup {
		return nil, mtproto.ErrChatAdminRequired
	}
	if !p.IsCreator() && !p.adminRights().CanChangeInfo() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	f.HiddenPrehistory = enabled
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, self, mtproto.MakeTLChannelAdminLogEventActionTogglePreHistoryHidden(&mtproto.ChannelAdminLogEventAction{
		NewValue_BOOL: mtproto.ToBool(enabled),
	}).To_ChannelAdminLogEventAction())
	return s.pack(ctx, self, ch.ID, channelUpdate(ch.ID)), nil
}

func (s *Service) ToggleParticipantsHidden(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, enabled bool) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	f.ParticipantsHidden = enabled
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return s.pack(ctx, self, ch.ID, channelUpdate(ch.ID)), nil
}

func (s *Service) ToggleSlowMode(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, seconds int32) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !ch.Megagroup {
		return nil, mtproto.ErrChatAdminRequired
	}
	if !p.IsCreator() && !p.adminRights().CanChangeInfo() && !p.adminRights().CanBanUsers() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if !validSlowmode(seconds) {
		return nil, mtproto.ErrSlowmodeMultiMsgsDisabled
	}
	f := s.features(ctx, ch.ID)
	prev := f.SlowmodeSeconds
	f.SlowmodeSeconds = seconds
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, self, mtproto.MakeTLChannelAdminLogEventActionToggleSlowMode(&mtproto.ChannelAdminLogEventAction{
		PrevValue_INT32: prev,
		NewValue_INT32:  seconds,
	}).To_ChannelAdminLogEventAction())
	return s.pack(ctx, self, ch.ID, channelUpdate(ch.ID)), nil
}

func validSlowmode(seconds int32) bool {
	switch seconds {
	case 0, 10, 30, 60, 300, 900, 3600:
		return true
	default:
		return false
	}
}

func (s *Service) ToggleJoinToSend(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, enabled bool) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsCreator() && !p.adminRights().CanInviteUsers() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	f.JoinToSend = enabled
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return s.pack(ctx, self, ch.ID, channelUpdate(ch.ID)), nil
}

func (s *Service) ToggleJoinRequest(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, enabled bool) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsCreator() && !p.adminRights().CanInviteUsers() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	f.JoinRequest = enabled
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return s.pack(ctx, self, ch.ID, channelUpdate(ch.ID)), nil
}

func (s *Service) ApproveJoin(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, userID int64, approve bool) error {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return err
	}
	if !p.IsAdmin() {
		return mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	kept := f.JoinRequests[:0]
	found := false
	for _, req := range f.JoinRequests {
		if req.UserID == userID {
			found = true
			continue
		}
		kept = append(kept, req)
	}
	f.JoinRequests = kept
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return err
	}
	if !found {
		return mtproto.ErrUserIdInvalid
	}
	if approve {
		_, err = s.Invite(ctx, self, actorAuth, in, []int64{userID}, nil)
		return err
	}
	return nil
}

// HideAllJoins implements messages.hideAllChatJoinRequests for a channel.
// link limits the dismiss to requests that arrived through that invite.
// https://core.telegram.org/method/messages.hideAllChatJoinRequests
func (s *Service) HideAllJoins(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, approved bool, link string) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsAdmin() && !p.adminRights().CanInviteUsers() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	kept := make([]JoinReq, 0, len(f.JoinRequests))
	var hit []int64
	want := brand.InviteHash(link)
	for _, req := range f.JoinRequests {
		if want != "" {
			stored := brand.InviteHash(req.Link)
			if stored == "" || (stored != want && req.Link != link) {
				kept = append(kept, req)
				continue
			}
		}
		hit = append(hit, req.UserID)
	}
	f.JoinRequests = kept
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	if approved && len(hit) > 0 {
		if _, err := s.Invite(ctx, self, actorAuth, in, hit, nil); err != nil {
			return nil, err
		}
	}
	s.syncImportersAfterHide(ctx, self, ch.ID, hit, approved, link)
	recent := make([]int64, 0, len(kept))
	for _, req := range kept {
		recent = append(recent, req.UserID)
		if len(recent) == 5 {
			break
		}
	}
	up := mtproto.MakeTLUpdatePendingJoinRequests(&mtproto.Update{
		Peer_PEER:        mtproto.MakePeerChannel(ch.ID),
		RequestsPending:  int32(len(kept)),
		RecentRequesters: recent,
	}).To_Update()
	return s.pack(ctx, self, ch.ID, up), nil
}

// SetSponsoredRestricted stores channels.restrictSponsoredMessages.
// https://core.telegram.org/method/channels.restrictSponsoredMessages
func (s *Service) SetSponsoredRestricted(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, restricted bool) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsCreator() && !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	_ = actorAuth
	f := s.features(ctx, ch.ID)
	f.SponsoredRestricted = restricted
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return s.pack(ctx, self, ch.ID, channelUpdate(ch.ID)), nil
}

// SponsoredRestricted reports the flag stored by SetSponsoredRestricted.
func (s *Service) SponsoredRestricted(ctx context.Context, channelID int64) bool {
	return s.features(ctx, channelID).SponsoredRestricted
}

func (s *Service) EditLocation(ctx context.Context, self int64, in *mtproto.InputChannel, lat, long float64, address string) error {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return err
	}
	if !p.IsCreator() && !p.adminRights().CanChangeInfo() {
		return mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	f.Lat = lat
	f.Long = long
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return err
	}
	ch.Address = address
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return err
	}
	point := mtproto.MakeTLGeoPoint(&mtproto.GeoPoint{Lat: lat, Long: long}).To_GeoPoint()
	s.logAdmin(ctx, ch.ID, self, mtproto.MakeTLChannelAdminLogEventActionChangeLocation(&mtproto.ChannelAdminLogEventAction{
		NewValue_CHANNELLOCATION: mtproto.MakeTLChannelLocation(&mtproto.ChannelLocation{
			GeoPoint: point,
			Address:  address,
		}).To_ChannelLocation(),
	}).To_ChannelAdminLogEventAction())
	return nil
}

func (s *Service) EditCreator(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, userID int64, password *mtproto.InputCheckPasswordSRP) (*mtproto.Updates, error) {
	// Cloud password verifiers are not stored. An empty proof is the
	// documented PASSWORD_MISSING (2FA required). A submitted SRP A is the
	// same acceptance this method already used.
	// https://core.telegram.org/method/messages.editChatCreator
	if password == nil || len(password.GetA()) == 0 || password.GetPredicateName() == mtproto.Predicate_inputCheckPasswordEmpty {
		return nil, mtproto.ErrPasswordMissing
	}
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	target, err := s.store.GetParticipant(ctx, ch.ID, userID)
	if err != nil {
		return nil, err
	}
	if target == nil || !target.InChannel() {
		return nil, mtproto.ErrUserNotParticipant
	}
	p.Role = RoleAdmin
	p.AdminRights = allAdminRights()
	target.Role = RoleCreator
	target.AdminRights = allAdminRights()
	if err := s.store.PutParticipant(ctx, p); err != nil {
		return nil, err
	}
	if err := s.store.PutParticipant(ctx, target); err != nil {
		return nil, err
	}
	ch.CreatorID = userID
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	f.FutureCreatorID = 0
	_ = s.saveFeatures(ctx, ch.ID, f)
	s.logAdmin(ctx, ch.ID, self, mtproto.MakeTLChannelAdminLogEventActionParticipantToggleAdmin(&mtproto.ChannelAdminLogEventAction{
		NewParticipant:  participantTL(self, target),
		PrevParticipant: participantTL(self, p),
	}).To_ChannelAdminLogEventAction())
	return s.pack(ctx, self, ch.ID, channelUpdate(ch.ID)), nil
}

func (s *Service) FutureCreator(ctx context.Context, self int64, in *mtproto.InputChannel) (int64, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return 0, err
	}
	if !p.IsCreator() {
		return 0, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	if f.FutureCreatorID != 0 {
		return f.FutureCreatorID, nil
	}
	parts, err := s.store.ListParticipants(ctx, ch.ID)
	if err != nil {
		return 0, err
	}
	var adminID, memberID int64
	var adminDate, memberDate int32 = 1<<31 - 1, 1<<31 - 1
	for _, part := range parts {
		if part.UserID == self || !part.InChannel() {
			continue
		}
		if part.IsAdmin() && part.Date < adminDate {
			adminDate = part.Date
			adminID = part.UserID
		}
		if part.Date < memberDate {
			memberDate = part.Date
			memberID = part.UserID
		}
	}
	if adminID != 0 {
		return adminID, nil
	}
	if memberID == 0 {
		return 0, mtproto.ErrChatAdminRequired
	}
	return memberID, nil
}

func (s *Service) SetFutureCreator(ctx context.Context, self int64, in *mtproto.InputChannel, userID int64) error {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return err
	}
	if !p.IsCreator() {
		return mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	f.FutureCreatorID = userID
	return s.saveFeatures(ctx, ch.ID, f)
}

func (s *Service) GroupsForDiscussion(ctx context.Context, self int64) (*mtproto.Messages_Chats, error) {
	list, err := s.store.ListByUser(ctx, self)
	if err != nil {
		return nil, err
	}
	var chats []*mtproto.Chat
	for _, ch := range list {
		if !ch.Megagroup {
			continue
		}
		p, _ := s.store.GetParticipant(ctx, ch.ID, self)
		if p.IsAdmin() {
			chats = append(chats, s.chatFor(ctx, self, ch, p))
		}
	}
	if chats == nil {
		chats = []*mtproto.Chat{}
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
}

func (s *Service) SetDiscussionGroup(ctx context.Context, self int64, broadcast, group *mtproto.InputChannel) error {
	bCh, bp, err := s.requireMember(ctx, self, broadcast)
	if err != nil {
		return err
	}
	if !bp.IsCreator() {
		return mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, bCh.ID)
	prev := f.LinkedChatID
	if group == nil || group.GetPredicateName() == mtproto.Predicate_inputChannelEmpty || group.GetChannelId() == 0 {
		f.LinkedChatID = 0
	} else {
		gCh, gp, err := s.requireMember(ctx, self, group)
		if err != nil {
			return err
		}
		if !gCh.Megagroup || !gp.IsAdmin() {
			return mtproto.ErrChatAdminRequired
		}
		f.LinkedChatID = gCh.ID
		gf := s.features(ctx, gCh.ID)
		gf.LinkedChatID = bCh.ID
		if err := s.saveFeatures(ctx, gCh.ID, gf); err != nil {
			return err
		}
	}
	if err := s.saveFeatures(ctx, bCh.ID, f); err != nil {
		return err
	}
	s.logAdmin(ctx, bCh.ID, self, mtproto.MakeTLChannelAdminLogEventActionChangeLinkedChat(&mtproto.ChannelAdminLogEventAction{
		PrevValue_INT64: prev,
		NewValue_INT64:  f.LinkedChatID,
	}).To_ChannelAdminLogEventAction())
	return nil
}

func (s *Service) InactiveChannels(ctx context.Context, self int64) (*mtproto.Messages_InactiveChats, error) {
	list, err := s.store.ListByUser(ctx, self)
	if err != nil {
		return nil, err
	}
	now := int32(time.Now().Unix())
	var dates []int32
	var chats []*mtproto.Chat
	for _, ch := range list {
		date := ch.Date
		if msg, _ := s.store.GetMessage(ctx, ch.ID, ch.TopMessage); msg != nil {
			date = msg.Date
		}
		if now-date < 30*24*3600 && ch.TopMessage > 1 {
			continue
		}
		dates = append(dates, date)
		chats = append(chats, s.ChatFor(ctx, self, ch))
	}
	if dates == nil {
		dates = []int32{}
	}
	if chats == nil {
		chats = []*mtproto.Chat{}
	}
	return mtproto.MakeTLMessagesInactiveChats(&mtproto.Messages_InactiveChats{
		Dates: dates,
		Chats: chats,
		Users: []*mtproto.User{},
	}).To_Messages_InactiveChats(), nil
}

func (s *Service) ConvertToGigagroup(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !ch.Megagroup || !p.IsCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	if f.Gigagroup {
		return nil, mtproto.ErrChatNotModified
	}
	f.Gigagroup = true
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return s.pack(ctx, self, ch.ID, channelUpdate(ch.ID)), nil
}

func (s *Service) PersonalHistory(ctx context.Context, self, userID int64, limit, maxID, minID int32) (*mtproto.Messages_Messages, error) {
	if userID == 0 {
		userID = self
	}
	channelID, err := s.store.GetPersonalChannel(ctx, userID)
	if err != nil {
		return nil, err
	}
	if channelID == 0 {
		return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
			Messages: []*mtproto.Message{},
			Chats:    []*mtproto.Chat{},
			Users:    []*mtproto.User{},
		}).To_Messages_Messages(), nil
	}
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return nil, err
	}
	return s.history(ctx, self, ch, HistoryQuery{Limit: limit, MaxID: maxID, MinID: minID})
}

func (s *Service) SetPersonalChannel(ctx context.Context, self int64, in *mtproto.InputChannel) error {
	if in == nil || in.GetChannelId() == 0 {
		return s.store.SetPersonalChannel(ctx, self, 0)
	}
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return err
	}
	if !p.IsCreator() || !ch.Broadcast {
		return mtproto.ErrChatAdminRequired
	}
	return s.store.SetPersonalChannel(ctx, self, ch.ID)
}

// ChannelSearch is messages.search against one channel.
type ChannelSearch struct {
	Q        string
	FromUser int64
	OffsetID int32
	Limit    int32
	MinDate  int32
	MaxDate  int32
	MinID    int32
	MaxID    int32
	TopMsgID int32
	Filter   *mtproto.MessagesFilter
}

func (s *Service) SearchChannel(ctx context.Context, self, channelID int64, q string, fromUser int64, offsetID, limit int32) (*mtproto.Messages_Messages, error) {
	return s.SearchInChannel(ctx, self, channelID, ChannelSearch{
		Q: q, FromUser: fromUser, OffsetID: offsetID, Limit: limit,
	})
}

func (s *Service) SearchInChannel(ctx context.Context, self, channelID int64, q ChannelSearch) (*mtproto.Messages_Messages, error) {
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{
		PredicateName: mtproto.Predicate_inputChannel,
		ChannelId:     channelID,
	})
	if err != nil {
		return nil, err
	}
	q.Limit = pageLimit(q.Limit)
	text := strings.ToLower(q.Q)
	all, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ID > all[j].ID })
	feat := s.features(ctx, ch.ID)
	viewer, _ := s.store.GetParticipant(ctx, ch.ID, self)
	var msgs []*mtproto.Message
	for _, stored := range all {
		if stored == nil || stored.Deleted {
			continue
		}
		if q.OffsetID > 0 && stored.ID >= q.OffsetID {
			continue
		}
		if q.MinID > 0 && stored.ID <= q.MinID {
			continue
		}
		if q.MaxID > 0 && stored.ID >= q.MaxID {
			continue
		}
		if q.MinDate > 0 && stored.Date < q.MinDate {
			continue
		}
		if q.MaxDate > 0 && stored.Date > q.MaxDate {
			continue
		}
		if q.FromUser != 0 && stored.SenderID != q.FromUser {
			continue
		}
		if q.TopMsgID != 0 && !storedInTopic(feat, stored, q.TopMsgID) {
			continue
		}
		if prehistoryBlocks(feat, viewer, stored) {
			continue
		}
		body, err := unmarshalMessage(stored.Data)
		if err != nil {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(body.GetMessage()), text) {
			continue
		}
		if !channelFilterMatches(body, q.Filter, feat.PinnedMsgID == stored.ID) {
			continue
		}
		msgs = append(msgs, viewMessage(body, stored.SenderID, self))
		if int32(len(msgs)) >= q.Limit {
			break
		}
	}
	if msgs == nil {
		msgs = []*mtproto.Message{}
	}
	return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
		Pts:      ch.Pts,
		Count:    int32(len(msgs)),
		Messages: msgs,
		Chats:    []*mtproto.Chat{s.ChatFor(ctx, self, ch)},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

func (s *Service) SendAs(ctx context.Context, self int64) ([]*mtproto.Chat, []int64, error) {
	list, err := s.store.ListByUser(ctx, self)
	if err != nil {
		return nil, nil, err
	}
	var chats []*mtproto.Chat
	ids := []int64{self}
	for _, ch := range list {
		p, _ := s.store.GetParticipant(ctx, ch.ID, self)
		if p.IsAdmin() && (ch.Megagroup || p.adminRights().CanPostMessages() || p.IsCreator()) {
			chats = append(chats, s.chatFor(ctx, self, ch, p))
			ids = append(ids, ch.ID)
		}
	}
	return chats, ids, nil
}

func (s *Service) PinMessage(ctx context.Context, self, actorAuth, channelID int64, msgID int32, unpin bool) (*mtproto.Updates, error) {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return nil, err
	}
	p, err := s.store.GetParticipant(ctx, channelID, self)
	if err != nil {
		return nil, err
	}
	if p == nil || !p.InChannel() {
		return nil, mtproto.ErrChannelPrivate
	}
	if err = mtproto.MemberPinForbidden(int32(time.Now().Unix()), p.IsCreator() || p.adminRights().CanPinMessages(), memberBans(ch, p)); err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	if unpin {
		f.PinnedMsgID = 0
	} else {
		stored, err := s.store.GetMessage(ctx, channelID, msgID)
		if err != nil || stored == nil || stored.Deleted {
			return nil, mtproto.ErrMessageIdInvalid
		}
		f.PinnedMsgID = msgID
		body, _ := unmarshalMessage(stored.Data)
		if body != nil {
			body.Pinned = true
			raw, _ := proto.Marshal(body)
			stored.Data = raw
			_, _ = s.store.CommitEdit(ctx, channelID, msgID, int32(time.Now().Unix()), raw)
		}
	}
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	action := mtproto.MakeTLMessageActionPinMessage(&mtproto.MessageAction{}).To_MessageAction()
	msg := serviceMessage(self, channelID, action)
	if _, _, err = s.appendMessage(ctx, channelID, self, 0, msg); err != nil {
		return nil, err
	}
	return s.actorUpdates(ctx, self, actorAuth, channelID, KindNew)
}

// UnpinAll clears the channel's pinned message. A channel with nothing
// pinned returns the current pts and a zero count.
func (s *Service) UnpinAll(ctx context.Context, self, actorAuth, channelID int64) (int32, int32, error) {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return 0, 0, err
	}
	p, err := s.store.GetParticipant(ctx, channelID, self)
	if err != nil {
		return 0, 0, err
	}
	if p == nil || !p.InChannel() {
		return 0, 0, mtproto.ErrChannelPrivate
	}
	if err = mtproto.MemberPinForbidden(int32(time.Now().Unix()), p.IsCreator() || p.adminRights().CanPinMessages(), memberBans(ch, p)); err != nil {
		return 0, 0, err
	}
	f := s.features(ctx, ch.ID)
	if f.PinnedMsgID == 0 {
		return ch.Pts, 0, nil
	}
	pinned := f.PinnedMsgID
	f.PinnedMsgID = 0
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return 0, 0, err
	}
	if stored, err := s.store.GetMessage(ctx, channelID, pinned); err == nil && stored != nil && !stored.Deleted {
		if body, _ := unmarshalMessage(stored.Data); body != nil {
			body.Pinned = false
			raw, _ := proto.Marshal(body)
			_, _ = s.store.CommitEdit(ctx, channelID, pinned, int32(time.Now().Unix()), raw)
		}
	}
	_ = actorAuth
	return ch.Pts, 1, nil
}

func (s *Service) CreateTopic(ctx context.Context, self, actorAuth, channelID int64, title string, iconColor int32, iconEmoji int64) (*mtproto.Updates, *TopicRec, error) {
	ch, p, err := s.requireMember(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, nil, err
	}
	if !ch.Forum {
		return nil, nil, mtproto.ErrChatAdminRequired
	}
	if !p.InChannel() {
		return nil, nil, mtproto.ErrChannelPrivate
	}
	if err = mtproto.MemberTopicsForbidden(int32(time.Now().Unix()), p.IsCreator() || p.adminRights().CanManageTopics(), memberBans(ch, p)); err != nil {
		return nil, nil, err
	}
	if title == "" {
		return nil, nil, mtproto.ErrChatTitleEmpty
	}
	f := s.features(ctx, ch.ID)
	topic := TopicRec{
		Title:     title,
		IconColor: iconColor,
		IconEmoji: iconEmoji,
		CreatorID: self,
		Date:      int32(time.Now().Unix()),
	}
	msg := serviceMessage(self, channelID, mtproto.MakeTLMessageActionTopicCreate(&mtproto.MessageAction{
		Title:        title,
		Title_STRING: title,
	}).To_MessageAction())
	id, _, err := s.appendMessage(ctx, channelID, self, 0, msg)
	if err != nil {
		return nil, nil, err
	}
	// forumTopic.id is the service message that created the topic.
	topic.ID = id
	topic.TopMessage = id
	if f.MsgTopic == nil {
		f.MsgTopic = map[int32]int32{}
	}
	f.MsgTopic[id] = topic.ID
	f.Topics = append(f.Topics, topic)
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, nil, err
	}
	up, err := s.actorUpdates(ctx, self, actorAuth, channelID, KindNew)
	return up, &topic, err
}

func (s *Service) ListTopics(ctx context.Context, self, channelID int64, q string, limit, offsetDate, offsetID, offsetTopic int32) ([]*mtproto.ForumTopic, *mtproto.Chat, error) {
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, nil, err
	}
	f := s.features(ctx, ch.ID)
	q = strings.ToLower(q)
	if limit <= 0 {
		limit = 20
	}
	rows := append([]TopicRec(nil), f.Topics...)
	sortTopics(rows)
	start := 0
	if offsetTopic != 0 {
		for i, t := range rows {
			if t.ID == offsetTopic {
				start = i + 1
				break
			}
		}
	} else if offsetDate != 0 || offsetID != 0 {
		for i, t := range rows {
			if t.Pinned {
				continue
			}
			if t.Date < offsetDate || (t.Date == offsetDate && t.ID < offsetID) {
				start = i
				break
			}
			start = i + 1
		}
	}
	if start > len(rows) {
		start = len(rows)
	}
	var topics []*mtproto.ForumTopic
	for _, t := range rows[start:] {
		if q != "" && !strings.Contains(strings.ToLower(t.Title), q) {
			continue
		}
		topics = append(topics, s.topicTL(self, ch.ID, t))
		if int32(len(topics)) >= limit {
			break
		}
	}
	return topics, s.ChatFor(ctx, self, ch), nil
}

func sortTopics(rows []TopicRec) {
	for i := 0; i < len(rows); i++ {
		for j := i + 1; j < len(rows); j++ {
			if topicLess(rows[j], rows[i]) {
				rows[i], rows[j] = rows[j], rows[i]
			}
		}
	}
}

func topicLess(a, b TopicRec) bool {
	if a.Pinned != b.Pinned {
		return a.Pinned
	}
	if a.Pinned && b.Pinned && a.PinOrder != b.PinOrder {
		return a.PinOrder < b.PinOrder
	}
	if a.Date != b.Date {
		return a.Date > b.Date
	}
	return a.ID > b.ID
}

func (s *Service) topicTL(self, channelID int64, t TopicRec) *mtproto.ForumTopic {
	ft := mtproto.MakeTLForumTopic(&mtproto.ForumTopic{
		My:             t.CreatorID == self,
		Closed:         t.Closed,
		Pinned:         t.Pinned,
		Hidden:         t.Hidden,
		Id:             t.ID,
		Date:           t.Date,
		Peer:           mtproto.MakePeerChannel(channelID),
		Title:          t.Title,
		IconColor:      t.IconColor,
		TopMessage:     t.TopMessage,
		FromId:         mtproto.MakePeerUser(t.CreatorID),
		NotifySettings: defaultNotify(),
	}).To_ForumTopic()
	if t.IconEmoji != 0 {
		ft.IconEmojiId = &wrapperspb.Int64Value{Value: t.IconEmoji}
	}
	return ft
}

func (s *Service) EditTopic(ctx context.Context, self, channelID int64, topicID int32, title string, closed *bool, iconEmoji *int64, hidden *bool) (*TopicRec, error) {
	ch, p, err := s.requireMember(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	idx, ok := findTopic(f, topicID)
	if !ok {
		return nil, mtproto.ErrTopicIdInvalid
	}
	if f.Topics[idx].CreatorID != self && !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if title != "" {
		f.Topics[idx].Title = title
	}
	if closed != nil {
		f.Topics[idx].Closed = *closed
	}
	if iconEmoji != nil {
		f.Topics[idx].IconEmoji = *iconEmoji
	}
	if hidden != nil {
		if !p.IsAdmin() && f.Topics[idx].CreatorID != self {
			return nil, mtproto.ErrChatAdminRequired
		}
		f.Topics[idx].Hidden = *hidden
	}
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	cp := f.Topics[idx]
	return &cp, nil
}

func findTopic(f *Features, id int32) (int, bool) {
	if f == nil || id == 0 {
		return 0, false
	}
	for i := range f.Topics {
		if f.Topics[i].ID == id {
			return i, true
		}
	}
	return 0, false
}

func (s *Service) PinTopic(ctx context.Context, self, channelID int64, topicID int32, pinned bool) error {
	ch, p, err := s.requireMember(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return err
	}
	if !p.IsAdmin() {
		return mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	idx, ok := findTopic(f, topicID)
	if !ok {
		return mtproto.ErrTopicIdInvalid
	}
	f.Topics[idx].Pinned = pinned
	if pinned {
		var max int32
		for _, t := range f.Topics {
			if t.Pinned && t.PinOrder >= max {
				max = t.PinOrder + 1
			}
		}
		f.Topics[idx].PinOrder = max
	}
	return s.saveFeatures(ctx, ch.ID, f)
}

// ReorderPinnedTopics applies messages.reorderPinnedForumTopics.
// order is the pinned topic ids, first in the list shown first.
// force unpins topics that are missing from order.
func (s *Service) ReorderPinnedTopics(ctx context.Context, self, channelID int64, order []int32, force bool) error {
	ch, p, err := s.requireMember(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return err
	}
	if !p.IsAdmin() {
		return mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	seen := map[int32]int32{}
	for i, id := range order {
		idx, ok := findTopic(f, id)
		if !ok {
			return mtproto.ErrTopicIdInvalid
		}
		seen[f.Topics[idx].ID] = int32(i)
	}
	if !force {
		for _, t := range f.Topics {
			if t.Pinned {
				if _, ok := seen[t.ID]; !ok {
					return mtproto.ErrTopicIdInvalid
				}
			}
		}
	}
	for i := range f.Topics {
		pos, ok := seen[f.Topics[i].ID]
		if ok {
			f.Topics[i].Pinned = true
			f.Topics[i].PinOrder = pos
			continue
		}
		if force {
			f.Topics[i].Pinned = false
		}
	}
	return s.saveFeatures(ctx, ch.ID, f)
}

// DeleteTopicHistory deletes messages that belong to one forum topic.
// It does not clear the rest of the channel. The topic record stays so the
// topic id (the creation message) remains addressable.
func (s *Service) DeleteTopicHistory(ctx context.Context, self, actorAuth, channelID int64, topMsgID int32) (pts, ptsCount int32, err error) {
	ch, p, err := s.requireMember(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return 0, 0, err
	}
	if !ch.Forum {
		return 0, 0, mtproto.ErrTopicIdInvalid
	}
	f := s.features(ctx, ch.ID)
	idx, ok := findTopic(f, topMsgID)
	if !ok {
		return 0, 0, mtproto.ErrTopicIdInvalid
	}
	if !p.IsCreator() && !p.adminRights().CanDeleteMessages() {
		return 0, 0, mtproto.ErrChatAdminRequired
	}
	topicID := f.Topics[idx].ID
	msgs, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return 0, 0, err
	}
	var ids []int32
	for _, m := range msgs {
		if m.Deleted || m.ID == topicID {
			continue
		}
		if messageInTopic(f, m, topicID) {
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		return ch.Pts, 0, nil
	}
	pts, ptsCount, err = s.Delete(ctx, self, ch.ID, actorAuth, ids)
	if err != nil {
		return 0, 0, err
	}
	f = s.features(ctx, ch.ID)
	if i, ok := findTopic(f, topicID); ok {
		f.Topics[i].TopMessage = topicID
		for _, id := range ids {
			delete(f.MsgTopic, id)
		}
		_ = s.saveFeatures(ctx, ch.ID, f)
	}
	return pts, ptsCount, nil
}

func messageInTopic(f *Features, m *StoredMessage, topicID int32) bool {
	if f != nil && f.MsgTopic[m.ID] == topicID {
		return true
	}
	body, err := unmarshalMessage(m.Data)
	if err != nil || body == nil || body.GetReplyTo() == nil {
		return false
	}
	h := body.GetReplyTo()
	if h.GetReplyToTopId().GetValue() == topicID {
		return true
	}
	return h.GetReplyToMsgId() == topicID
}

// ensureGeneralTopic creates the hidden General topic once when a forum is enabled.
func (s *Service) ensureGeneralTopic(ctx context.Context, self, channelID int64) {
	f := s.features(ctx, channelID)
	for _, t := range f.Topics {
		if t.Hidden && strings.EqualFold(t.Title, "General") {
			return
		}
	}
	_, topic, err := s.CreateTopic(ctx, self, 0, channelID, "General", 0x6FB9F0, 0)
	if err != nil || topic == nil {
		return
	}
	hidden := true
	_, _ = s.EditTopic(ctx, self, channelID, topic.ID, "", nil, nil, &hidden)
}

// bindTopic records which forum topic a new message belongs to and advances top_message.
func (s *Service) bindTopic(ctx context.Context, channelID int64, msgID int32, msg *mtproto.Message) {
	if msg == nil || msg.GetReplyTo() == nil {
		return
	}
	h := msg.GetReplyTo()
	top := h.GetReplyToTopId().GetValue()
	if top == 0 {
		top = h.GetReplyToMsgId()
	}
	if top == 0 {
		return
	}
	f := s.features(ctx, channelID)
	idx, ok := findTopic(f, top)
	if !ok {
		return
	}
	if f.MsgTopic == nil {
		f.MsgTopic = map[int32]int32{}
	}
	f.MsgTopic[msgID] = f.Topics[idx].ID
	if msgID > f.Topics[idx].TopMessage {
		f.Topics[idx].TopMessage = msgID
	}
	_ = s.saveFeatures(ctx, channelID, f)
}

func (s *Service) ToggleForum(ctx context.Context, self int64, in *mtproto.InputChannel, enabled bool) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !ch.Megagroup || !p.IsCreator() {
		return nil, mtproto.ErrChatAdminRequired
	}
	ch.Forum = enabled
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	if enabled {
		s.ensureGeneralTopic(ctx, self, ch.ID)
	}
	return s.pack(ctx, self, ch.ID, channelUpdate(ch.ID)), nil
}

func (s *Service) React(ctx context.Context, self, channelID int64, msgID int32, emoticons []string, big bool) (*mtproto.Updates, error) {
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, err
	}
	stored, err := s.store.GetMessage(ctx, channelID, msgID)
	if err != nil || stored == nil || stored.Deleted {
		return nil, mtproto.ErrMessageIdInvalid
	}
	f := s.features(ctx, ch.ID)
	if err := allowReact(f, emoticons); err != nil {
		return nil, err
	}
	kept := f.Reactions[:0]
	for _, r := range f.Reactions {
		if !(r.MsgID == msgID && r.UserID == self) {
			kept = append(kept, r)
		}
	}
	f.Reactions = kept
	now := int32(time.Now().Unix())
	for _, emo := range emoticons {
		if emo == "" {
			continue
		}
		f.Reactions = append(f.Reactions, ReactRec{MsgID: msgID, UserID: self, Emoticon: emo, Date: now, Big: big})
	}
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	// A pts event lets channels.getDifference replay the chip after the
	// client already has the message. History reads the same feature rows.
	if _, err = s.store.AppendPts(ctx, ch.ID, KindReact, msgID, nil); err != nil {
		return nil, err
	}
	if reloaded, rerr := s.reload(ctx, ch.ID); rerr == nil && reloaded != nil {
		ch = reloaded
	}
	canSee, readAt := reactionView(ch, f, self)
	reactions := reactionSummary(f.Reactions, msgID, self, canSee, readAt)
	upd := mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
		Peer_PEER:                  mtproto.MakePeerChannel(channelID),
		MsgId_INT32:                msgID,
		Reactions_MESSAGEREACTIONS: reactions,
	}).To_Update()
	return s.updatesWith(ctx, self, ch, upd), nil
}

func reactionView(ch *Channel, f *Features, self int64) (canSee bool, readAt int32) {
	if f != nil && f.ReactionRead != nil {
		readAt = f.ReactionRead[self]
	}
	return ch != nil && ch.Megagroup, readAt
}

func reactionSummary(all []ReactRec, msgID int32, self int64, canSee bool, readAt int32) *mtproto.MessageReactions {
	counts := map[string]int32{}
	chosen := map[string]int32{}
	chosenSet := map[string]bool{}
	var order []string
	var selfN int32
	var mine []ReactRec
	for _, r := range all {
		if r.MsgID != msgID {
			continue
		}
		if _, ok := counts[r.Emoticon]; !ok {
			order = append(order, r.Emoticon)
		}
		counts[r.Emoticon]++
		if r.UserID == self && !chosenSet[r.Emoticon] {
			chosenSet[r.Emoticon] = true
			chosen[r.Emoticon] = selfN
			selfN++
		}
		mine = append(mine, r)
	}
	var results []*mtproto.ReactionCount
	for _, emo := range order {
		rc := mtproto.MakeTLReactionCount(&mtproto.ReactionCount{
			Reaction_REACTION: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emo}).To_Reaction(),
			Reaction_STRING:   emo,
			Count:             counts[emo],
		}).To_ReactionCount()
		if chosenSet[emo] {
			rc.ChosenOrder = &wrapperspb.Int32Value{Value: chosen[emo]}
		}
		results = append(results, rc)
	}
	if results == nil {
		results = []*mtproto.ReactionCount{}
	}
	out := &mtproto.MessageReactions{Results: results}
	// Broadcast channels must not name reactors. Megagroups can.
	// https://core.telegram.org/method/messages.getMessageReactionsList
	if canSee && len(results) > 0 {
		out.CanSeeList = true
		out.RecentReactions = recentChannelReactions(mine, self, readAt, 3)
	}
	return mtproto.MakeTLMessageReactions(out).To_MessageReactions()
}

// attachReactions copies stored channel reactions onto history and difference
// messages. An empty summary is left unset so a cleared reaction does not
// leave a chip on the bubble.
func (s *Service) attachReactions(ctx context.Context, viewer, channelID int64, msgs []*mtproto.Message) {
	if len(msgs) == 0 {
		return
	}
	ch, err := s.reload(ctx, channelID)
	if err != nil || ch == nil {
		return
	}
	f := s.features(ctx, channelID)
	canSee, readAt := reactionView(ch, f, viewer)
	for _, m := range msgs {
		if m == nil || m.GetId() == 0 {
			continue
		}
		sum := reactionSummary(f.Reactions, m.GetId(), viewer, canSee, readAt)
		if len(sum.GetResults()) == 0 {
			continue
		}
		m.Reactions = sum
	}
}

func (s *Service) channelReactionUpdate(ctx context.Context, viewer int64, ch *Channel, msgID int32) *mtproto.Update {
	f := s.features(ctx, ch.ID)
	canSee, readAt := reactionView(ch, f, viewer)
	return mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
		Peer_PEER:                  mtproto.MakePeerChannel(ch.ID),
		MsgId_INT32:                msgID,
		Reactions_MESSAGEREACTIONS: reactionSummary(f.Reactions, msgID, viewer, canSee, readAt),
	}).To_Update()
}

func (s *Service) ReactionsFor(ctx context.Context, self, channelID int64, ids []int32) (*mtproto.Updates, error) {
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	f := s.features(ctx, ch.ID)
	canSee, readAt := reactionView(ch, f, self)
	var ups []*mtproto.Update
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		stored, err := s.store.GetMessage(ctx, ch.ID, id)
		if err != nil {
			return nil, err
		}
		if stored == nil || stored.Deleted {
			continue
		}
		ups = append(ups, mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
			Peer_PEER:                  mtproto.MakePeerChannel(channelID),
			MsgId_INT32:                id,
			Reactions_MESSAGEREACTIONS: reactionSummary(f.Reactions, id, self, canSee, readAt),
		}).To_Update())
	}
	if len(ups) == 0 {
		return nil, mtproto.ErrMsgIdInvalid
	}
	return s.updatesWith(ctx, self, ch, ups...), nil
}

func (s *Service) Replies(ctx context.Context, self, channelID int64, msgID int32, q HistoryQuery) (*mtproto.Messages_Messages, error) {
	if commentsOn() {
		if out, ok, err := s.repliesFromComments(ctx, self, channelID, msgID, q); ok {
			return out, err
		}
	}
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	targets := []int64{ch.ID}
	if f.LinkedChatID != 0 {
		targets = append(targets, f.LinkedChatID)
	}
	var msgs []*mtproto.Message
	var chats []*mtproto.Chat
	for _, id := range targets {
		peerCh, err := s.reload(ctx, id)
		if err != nil {
			continue
		}
		chats = append(chats, s.ChatFor(ctx, self, peerCh))
		all, _ := s.store.ListMessages(ctx, id)
		peerFeat := s.features(ctx, id)
		peerViewer, _ := s.store.GetParticipant(ctx, id, self)
		topicID := msgID
		if i, ok := findTopic(f, msgID); ok {
			topicID = f.Topics[i].ID
		}
		for _, stored := range all {
			if stored == nil || stored.Deleted {
				continue
			}
			if q.MinID > 0 && stored.ID <= q.MinID {
				continue
			}
			if q.MaxID > 0 && stored.ID >= q.MaxID {
				continue
			}
			if prehistoryBlocks(peerFeat, peerViewer, stored) {
				continue
			}
			body, err := unmarshalMessage(stored.Data)
			if err != nil || !messageInTopic(f, stored, topicID) {
				continue
			}
			msgs = append(msgs, viewMessage(body, stored.SenderID, self))
		}
	}
	total := int32(len(msgs))
	msgs = pageDescAt(msgs, q.OffsetID, q.OffsetDate, q.AddOffset, q.Limit)
	return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
		Pts: ch.Pts, Count: total, Messages: nonempty(msgs), Chats: chats, Users: []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

func (s *Service) DiscussionMessage(ctx context.Context, self, channelID int64, msgID int32) (*mtproto.Messages_DiscussionMessage, error) {
	if commentsOn() {
		if out, ok, err := s.discussionFromComments(ctx, self, channelID, msgID); ok {
			return out, err
		}
	}
	_, _, err := s.requireView(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, err
	}
	stored, err := s.store.GetMessage(ctx, channelID, msgID)
	if err != nil || stored == nil || stored.Deleted {
		return nil, mtproto.ErrMessageIdInvalid
	}
	viewer, _ := s.store.GetParticipant(ctx, channelID, self)
	if prehistoryBlocks(s.features(ctx, channelID), viewer, stored) {
		return nil, mtproto.ErrMessageIdInvalid
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return nil, err
	}
	replies, err := s.Replies(ctx, self, channelID, msgID, HistoryQuery{Limit: 20})
	if err != nil {
		return nil, err
	}
	msgs := append([]*mtproto.Message{viewMessage(body, stored.SenderID, self)}, replies.Messages...)
	var maxID int32
	for _, m := range replies.Messages {
		if m.Id > maxID {
			maxID = m.Id
		}
	}
	return mtproto.MakeTLMessagesDiscussionMessage(&mtproto.Messages_DiscussionMessage{
		Messages:       msgs,
		MaxId:          &wrapperspb.Int32Value{Value: maxID},
		ReadInboxMaxId: &wrapperspb.Int32Value{Value: maxID},
		UnreadCount:    0,
		Chats:          replies.Chats,
		Users:          []*mtproto.User{},
	}).To_Messages_DiscussionMessage(), nil
}

func (s *Service) BroadcastStats(ctx context.Context, self int64, in *mtproto.InputChannel) (*mtproto.Stats_BroadcastStats, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	return s.broadcastStats(ctx, ch), nil
}

func (s *Service) MegagroupStats(ctx context.Context, self int64, in *mtproto.InputChannel) (*mtproto.Stats_MegagroupStats, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	msgs, _ := s.store.ListMessages(ctx, ch.ID)
	var live int32
	for _, m := range msgs {
		if !m.Deleted {
			live++
		}
	}
	abs := func(v int32) *mtproto.StatsAbsValueAndPrev {
		return mtproto.MakeTLStatsAbsValueAndPrev(&mtproto.StatsAbsValueAndPrev{Current: float64(v), Previous: float64(v)}).To_StatsAbsValueAndPrev()
	}
	graph := func() *mtproto.StatsGraph {
		return mtproto.MakeTLStatsGraphAsync(&mtproto.StatsGraph{Token: "0"}).To_StatsGraph()
	}
	now := int32(time.Now().Unix())
	return mtproto.MakeTLStatsMegagroupStats(&mtproto.Stats_MegagroupStats{
		Period:                  mtproto.MakeTLStatsDateRangeDays(&mtproto.StatsDateRangeDays{MinDate: ch.Date, MaxDate: now}).To_StatsDateRangeDays(),
		Members:                 abs(ch.ParticipantsCount),
		Messages:                abs(live),
		Viewers:                 abs(ch.ParticipantsCount),
		Posters:                 abs(1),
		GrowthGraph:             graph(),
		MembersGraph:            graph(),
		NewMembersBySourceGraph: graph(),
		LanguagesGraph:          graph(),
		MessagesGraph:           graph(),
		ActionsGraph:            graph(),
		TopHoursGraph:           graph(),
		WeekdaysGraph:           graph(),
		TopPosters:              []*mtproto.StatsGroupTopPoster{},
		TopAdmins:               []*mtproto.StatsGroupTopAdmin{},
		TopInviters:             []*mtproto.StatsGroupTopInviter{},
		Users:                   []*mtproto.User{},
	}).To_Stats_MegagroupStats(), nil
}

func (s *Service) broadcastStats(ctx context.Context, ch *Channel) *mtproto.Stats_BroadcastStats {
	msgs, _ := s.store.ListMessages(ctx, ch.ID)
	var posts, views int32
	for _, m := range msgs {
		if m.Deleted {
			continue
		}
		posts++
		body, err := unmarshalMessage(m.Data)
		if err == nil && body.GetViews() != nil {
			views += body.GetViews().GetValue()
		}
	}
	per := float64(0)
	if posts > 0 {
		per = float64(views) / float64(posts)
	}
	abs := func(cur float64) *mtproto.StatsAbsValueAndPrev {
		return mtproto.MakeTLStatsAbsValueAndPrev(&mtproto.StatsAbsValueAndPrev{Current: cur, Previous: cur}).To_StatsAbsValueAndPrev()
	}
	graph := func() *mtproto.StatsGraph {
		return mtproto.MakeTLStatsGraphAsync(&mtproto.StatsGraph{Token: "0"}).To_StatsGraph()
	}
	now := int32(time.Now().Unix())
	f := s.features(ctx, ch.ID)
	var reactCount int32
	for range f.Reactions {
		reactCount++
	}
	reactPer := float64(0)
	if posts > 0 {
		reactPer = float64(reactCount) / float64(posts)
	}
	return mtproto.MakeTLStatsBroadcastStats(&mtproto.Stats_BroadcastStats{
		Period:                       mtproto.MakeTLStatsDateRangeDays(&mtproto.StatsDateRangeDays{MinDate: ch.Date, MaxDate: now}).To_StatsDateRangeDays(),
		Followers:                    abs(float64(ch.ParticipantsCount)),
		ViewsPerPost:                 abs(per),
		SharesPerPost:                abs(0),
		ReactionsPerPost:             abs(reactPer),
		ViewsPerStory:                abs(0),
		SharesPerStory:               abs(0),
		ReactionsPerStory:            abs(0),
		EnabledNotifications:         mtproto.MakeTLStatsPercentValue(&mtproto.StatsPercentValue{Part: float64(ch.ParticipantsCount), Total: float64(ch.ParticipantsCount)}).To_StatsPercentValue(),
		GrowthGraph:                  graph(),
		FollowersGraph:               graph(),
		MuteGraph:                    graph(),
		TopHoursGraph:                graph(),
		InteractionsGraph:            graph(),
		IvInteractionsGraph:          graph(),
		ViewsBySourceGraph:           graph(),
		NewFollowersBySourceGraph:    graph(),
		LanguagesGraph:               graph(),
		ReactionsByEmotionGraph:      graph(),
		StoryInteractionsGraph:       graph(),
		StoryReactionsByEmotionGraph: graph(),
		RecentPostsInteractions:      []*mtproto.PostInteractionCounters{},
		RecentMessageInteractions:    []*mtproto.MessageInteractionCounters{},
	}).To_Stats_BroadcastStats()
}

// ResolveChatPhoto turns an InputChatPhoto into a stored photo id. uploaded is
// invoked for inputChatUploadedPhoto so the media service can persist the file.
func ResolveChatPhoto(ctx context.Context, owner int64, in *mtproto.InputChatPhoto, upload func(file, video *mtproto.InputFile, videoStart float64) (*mtproto.Photo, error), get func(photoID int64) (*mtproto.Photo, error)) (photoID int64, photo *mtproto.Photo, clear bool, err error) {
	if in == nil || in.GetPredicateName() == mtproto.Predicate_inputChatPhotoEmpty {
		return 0, mtproto.MakeTLPhotoEmpty(nil).To_Photo(), true, nil
	}
	switch in.GetPredicateName() {
	case mtproto.Predicate_inputChatUploadedPhoto:
		if upload == nil {
			return 0, nil, false, mtproto.ErrPhotoInvalid
		}
		var start float64
		if in.GetVideoStartTs() != nil {
			start = in.GetVideoStartTs().GetValue()
		}
		photo, err = upload(in.GetFile(), in.GetVideo(), start)
		if err != nil {
			return 0, nil, false, err
		}
		if photo == nil {
			return 0, nil, false, mtproto.ErrPhotoInvalid
		}
		return photo.GetId(), photo, false, nil
	case mtproto.Predicate_inputChatPhoto:
		id := in.GetId()
		if id == nil || id.GetPredicateName() == mtproto.Predicate_inputPhotoEmpty {
			return 0, mtproto.MakeTLPhotoEmpty(nil).To_Photo(), true, nil
		}
		if get != nil {
			photo, err = get(id.GetId())
			if err != nil {
				return 0, nil, false, err
			}
		}
		if photo == nil {
			photo = mtproto.MakeTLPhotoEmpty(&mtproto.Photo{Id: id.GetId()}).To_Photo()
		}
		return id.GetId(), photo, false, nil
	default:
		return 0, nil, false, mtproto.ErrInputRequestInvalid
	}
}

func (s *Service) ApplyPhoto(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, photoID int64, photo *mtproto.Photo) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !s.canChangeInfo(ch, p) {
		return nil, mtproto.ErrChatAdminRequired
	}
	var action *mtproto.MessageAction
	if photoID == 0 {
		ch.PhotoID = 0
		action = mtproto.MakeMessageActionChatDeletePhoto()
	} else {
		ch.PhotoID = photoID
		if photo == nil {
			photo = mtproto.MakeTLPhotoEmpty(&mtproto.Photo{Id: photoID}).To_Photo()
		}
		action = mtproto.MakeMessageActionChatEditPhoto(photo)
	}
	ch.Version++
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	msg := serviceMessage(self, ch.ID, action)
	if _, _, err = s.appendMessage(ctx, ch.ID, self, 0, msg); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, self, mtproto.MakeTLChannelAdminLogEventActionChangePhoto(&mtproto.ChannelAdminLogEventAction{
		NewPhoto: photo,
	}).To_ChannelAdminLogEventAction())
	return s.actorUpdates(ctx, self, actorAuth, ch.ID, KindNew)
}
