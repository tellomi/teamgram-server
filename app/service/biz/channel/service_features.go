package channel

import (
	"context"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
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
	if password == nil || len(password.GetA()) == 0 {
		return nil, mtproto.ErrPasswordHashInvalid
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

func (s *Service) SearchPosts(ctx context.Context, self int64, query, hashtag string, offsetID, limit int32) (*mtproto.Messages_Messages, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	needle := strings.ToLower(strings.TrimPrefix(query, "#"))
	if hashtag != "" {
		needle = strings.ToLower(strings.TrimPrefix(hashtag, "#"))
	}
	list, err := s.store.ListByUser(ctx, self)
	if err != nil {
		return nil, err
	}
	var msgs []*mtproto.Message
	var chats []*mtproto.Chat
	seen := map[int64]bool{}
	for _, ch := range list {
		if ch.Username == "" && !ch.Broadcast && !ch.Megagroup {
			continue
		}
		all, err := s.store.ListMessages(ctx, ch.ID)
		if err != nil {
			continue
		}
		for _, stored := range all {
			if stored.Deleted || (offsetID > 0 && stored.ID >= offsetID) {
				continue
			}
			body, err := unmarshalMessage(stored.Data)
			if err != nil {
				continue
			}
			text := strings.ToLower(body.GetMessage())
			if needle != "" && !strings.Contains(text, needle) {
				continue
			}
			msgs = append(msgs, viewMessage(body, stored.SenderID, self))
			if !seen[ch.ID] {
				seen[ch.ID] = true
				chats = append(chats, s.ChatFor(ctx, self, ch))
			}
			if int32(len(msgs)) >= limit {
				break
			}
		}
	}
	if msgs == nil {
		msgs = []*mtproto.Message{}
	}
	if chats == nil {
		chats = []*mtproto.Chat{}
	}
	return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
		Count:    int32(len(msgs)),
		Messages: msgs,
		Chats:    chats,
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

func (s *Service) SearchChannel(ctx context.Context, self, channelID int64, q string, fromUser int64, offsetID, limit int32) (*mtproto.Messages_Messages, error) {
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{
		PredicateName: mtproto.Predicate_inputChannel,
		ChannelId:     channelID,
	})
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	q = strings.ToLower(q)
	all, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	var msgs []*mtproto.Message
	for _, stored := range all {
		if stored.Deleted {
			continue
		}
		if offsetID > 0 && stored.ID >= offsetID {
			continue
		}
		if fromUser != 0 && stored.SenderID != fromUser {
			continue
		}
		body, err := unmarshalMessage(stored.Data)
		if err != nil {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(body.GetMessage()), q) {
			continue
		}
		msgs = append(msgs, viewMessage(body, stored.SenderID, self))
		if int32(len(msgs)) >= limit {
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
	if !p.IsCreator() && !p.adminRights().CanPinMessages() {
		return nil, mtproto.ErrChatAdminRequired
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
	if title == "" {
		return nil, nil, mtproto.ErrChatTitleEmpty
	}
	f := s.features(ctx, ch.ID)
	var next int32 = 1
	for _, t := range f.Topics {
		if t.ID >= next {
			next = t.ID + 1
		}
	}
	topic := TopicRec{
		ID:        next,
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

func (s *Service) ListTopics(ctx context.Context, self, channelID int64, q string, limit int32) ([]*mtproto.ForumTopic, *mtproto.Chat, error) {
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, nil, err
	}
	f := s.features(ctx, ch.ID)
	q = strings.ToLower(q)
	if limit <= 0 {
		limit = 20
	}
	var topics []*mtproto.ForumTopic
	for _, t := range f.Topics {
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

func (s *Service) EditTopic(ctx context.Context, self, channelID int64, topicID int32, title string, closed *bool) (*TopicRec, error) {
	ch, p, err := s.requireMember(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	for i := range f.Topics {
		if f.Topics[i].ID != topicID {
			continue
		}
		if f.Topics[i].CreatorID != self && !p.IsAdmin() {
			return nil, mtproto.ErrChatAdminRequired
		}
		if title != "" {
			f.Topics[i].Title = title
		}
		if closed != nil {
			f.Topics[i].Closed = *closed
		}
		if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
			return nil, err
		}
		cp := f.Topics[i]
		return &cp, nil
	}
	return nil, mtproto.ErrTopicIdInvalid
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
	for i := range f.Topics {
		if f.Topics[i].ID == topicID {
			f.Topics[i].Pinned = pinned
			return s.saveFeatures(ctx, ch.ID, f)
		}
	}
	return mtproto.ErrTopicIdInvalid
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
	reactions := reactionSummary(f.Reactions, msgID, self)
	upd := mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
		Peer_PEER:                  mtproto.MakePeerChannel(channelID),
		MsgId_INT32:                msgID,
		Reactions_MESSAGEREACTIONS: reactions,
	}).To_Update()
	return s.updatesWith(ctx, self, ch, upd), nil
}

func reactionSummary(all []ReactRec, msgID int32, self int64) *mtproto.MessageReactions {
	counts := map[string]int32{}
	chosen := map[string]bool{}
	var order []string
	for _, r := range all {
		if r.MsgID != msgID {
			continue
		}
		if _, ok := counts[r.Emoticon]; !ok {
			order = append(order, r.Emoticon)
		}
		counts[r.Emoticon]++
		if r.UserID == self {
			chosen[r.Emoticon] = true
		}
	}
	var results []*mtproto.ReactionCount
	for _, emo := range order {
		rc := mtproto.MakeTLReactionCount(&mtproto.ReactionCount{
			Reaction_REACTION: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: emo}).To_Reaction(),
			Count:             counts[emo],
		}).To_ReactionCount()
		if chosen[emo] {
			rc.ChosenOrder = &wrapperspb.Int32Value{Value: 0}
		}
		results = append(results, rc)
	}
	if results == nil {
		results = []*mtproto.ReactionCount{}
	}
	return mtproto.MakeTLMessageReactions(&mtproto.MessageReactions{
		CanSeeList: true,
		Results:    results,
	}).To_MessageReactions()
}

func (s *Service) ReactionsFor(ctx context.Context, self, channelID int64, ids []int32) (*mtproto.Updates, error) {
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	var ups []*mtproto.Update
	for _, id := range ids {
		ups = append(ups, mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
			Peer_PEER:                  mtproto.MakePeerChannel(channelID),
			MsgId_INT32:                id,
			Reactions_MESSAGEREACTIONS: reactionSummary(f.Reactions, id, self),
		}).To_Update())
	}
	return s.updatesWith(ctx, self, ch, ups...), nil
}

func (s *Service) Replies(ctx context.Context, self, channelID int64, msgID, offsetID, limit int32) (*mtproto.Messages_Messages, error) {
	ch, _, err := s.requireView(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	targets := []int64{ch.ID}
	if f.LinkedChatID != 0 {
		targets = append(targets, f.LinkedChatID)
	}
	if limit <= 0 || limit > 50 {
		limit = 20
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
		for _, stored := range all {
			if stored.Deleted || (offsetID > 0 && stored.ID >= offsetID) {
				continue
			}
			body, err := unmarshalMessage(stored.Data)
			if err != nil || body.GetReplyTo() == nil {
				continue
			}
			replyID := body.GetReplyTo().GetReplyToMsgId()
			if replyID == 0 {
				replyID = body.GetReplyTo().GetReplyToMsgId_INT32()
			}
			if replyID != msgID {
				continue
			}
			msgs = append(msgs, viewMessage(body, stored.SenderID, self))
			if int32(len(msgs)) >= limit {
				break
			}
		}
	}
	if msgs == nil {
		msgs = []*mtproto.Message{}
	}
	return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
		Pts: ch.Pts, Count: int32(len(msgs)), Messages: msgs, Chats: chats, Users: []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

func (s *Service) DiscussionMessage(ctx context.Context, self, channelID int64, msgID int32) (*mtproto.Messages_DiscussionMessage, error) {
	_, _, err := s.requireView(ctx, self, &mtproto.InputChannel{PredicateName: mtproto.Predicate_inputChannel, ChannelId: channelID})
	if err != nil {
		return nil, err
	}
	stored, err := s.store.GetMessage(ctx, channelID, msgID)
	if err != nil || stored == nil || stored.Deleted {
		return nil, mtproto.ErrMessageIdInvalid
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return nil, err
	}
	replies, err := s.Replies(ctx, self, channelID, msgID, 0, 20)
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
