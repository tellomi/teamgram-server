package channel

import (
	"context"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const (
	nameColorCount      = 8 // help.getPeerColors
	profileColorCount   = 6 // help.getPeerProfileColors
	maxActiveUsernames  = 10
	maxBoostsUnrestrict = 1_000_000
	leftChannelsPage    = 100
	recommendationCap   = 100
	reportCap           = 200
)

// PeerReport is one account.reportPeer, account.reportProfilePhoto, or
// messages.reportSpam record. Channel spam stays on the channel feature row.
type PeerReport struct {
	ReporterID int64
	PeerID     int64
	Kind       string
	Message    string
	PhotoID    int64
	Date       int32
}

func (s *Service) requireInfoAdmin(ctx context.Context, self int64, in *mtproto.InputChannel) (*Channel, *Participant, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, nil, err
	}
	if !p.IsCreator() && !p.adminRights().CanChangeInfo() {
		return nil, nil, mtproto.ErrChatAdminRequired
	}
	return ch, p, nil
}

func (s *Service) channelSettingUpdates(ctx context.Context, actor, actorAuth int64, channelID int64) *mtproto.Updates {
	up := s.pack(ctx, actor, channelID, channelUpdate(channelID))
	s.emit(ctx, actor, actorAuth, channelID, func(viewer int64) *mtproto.Updates {
		return s.pack(ctx, viewer, channelID, channelUpdate(channelID))
	})
	return up
}

func listedUsernames(ch *Channel, f *Features) []UsernameRec {
	if f != nil && len(f.Usernames) > 0 {
		return f.Usernames
	}
	if ch != nil && ch.Username != "" {
		return []UsernameRec{{Username: ch.Username, Active: true, Editable: true}}
	}
	return nil
}

func primaryUsername(list []UsernameRec) string {
	for _, u := range list {
		if u.Active {
			return u.Username
		}
	}
	return ""
}

func activeUsernames(list []UsernameRec) []string {
	var out []string
	for _, u := range list {
		if u.Active {
			out = append(out, u.Username)
		}
	}
	return out
}

func activeCount(list []UsernameRec) int {
	n := 0
	for _, u := range list {
		if u.Active {
			n++
		}
	}
	return n
}

func usernameVector(list []UsernameRec) []*mtproto.Username {
	if len(list) == 0 {
		return nil
	}
	out := make([]*mtproto.Username, 0, len(list))
	for _, u := range list {
		out = append(out, mtproto.MakeTLUsername(&mtproto.Username{
			Editable: u.Editable,
			Active:   u.Active,
			Username: u.Username,
		}).To_Username())
	}
	return out
}

func (s *Service) assignEditableUsername(ctx context.Context, actor int64, ch *Channel, username string) error {
	if username != "" {
		id, _, found, err := s.store.LookupUsername(ctx, username)
		if err != nil {
			return err
		}
		if found && id != ch.ID {
			return mtproto.ErrUsernameOccupied
		}
	}
	f := s.features(ctx, ch.ID)
	list := listedUsernames(ch, f)
	next := make([]UsernameRec, 0, len(list)+1)
	for _, u := range list {
		if u.Editable {
			continue
		}
		if username != "" && equalFold(u.Username, username) {
			return mtproto.ErrUsernameOccupied
		}
		next = append(next, u)
	}
	if username != "" {
		next = append([]UsernameRec{{Username: username, Active: true, Editable: true}}, next...)
	}
	if activeCount(next) > maxActiveUsernames {
		return mtproto.ErrUsernamesActiveTooMuch
	}
	prev := activeUsernames(list)
	f.Usernames = next
	ch.Username = primaryUsername(next)
	ch.Version++
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return err
	}
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return err
	}
	s.logUsernames(ctx, ch.ID, actor, prev, activeUsernames(next))
	return nil
}

func (s *Service) logUsernames(ctx context.Context, channelID, userID int64, prev, next []string) {
	if sameStrings(prev, next) {
		return
	}
	if prev == nil {
		prev = []string{}
	}
	if next == nil {
		next = []string{}
	}
	s.logAdmin(ctx, channelID, userID, mtproto.MakeTLChannelAdminLogEventActionChangeUsernames(&mtproto.ChannelAdminLogEventAction{
		PrevValue_VECTORSTRING: prev,
		NewValue_VECTORSTRING:  next,
	}).To_ChannelAdminLogEventAction())
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !equalFold(a[i], b[i]) {
			return false
		}
	}
	return true
}

// ToggleUsername activates or deactivates a username already on the channel.
// channels.toggleUsername#50f24105 channel:InputChannel username:string active:Bool = Bool;
func (s *Service) ToggleUsername(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, username string, active bool) (*mtproto.Bool, error) {
	ch, _, err := s.requireInfoAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	username = strings.TrimSpace(username)
	if username == "" || !validChannelUsername(username) {
		return nil, mtproto.ErrUsernameInvalid
	}
	f := s.features(ctx, ch.ID)
	list := append([]UsernameRec(nil), listedUsernames(ch, f)...)
	idx := -1
	for i, u := range list {
		if equalFold(u.Username, username) {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, mtproto.ErrUsernameInvalid
	}
	if list[idx].Active == active {
		return nil, mtproto.ErrUsernameNotModified
	}
	if active && activeCount(list) >= maxActiveUsernames {
		return nil, mtproto.ErrUsernamesActiveTooMuch
	}
	if active {
		id, _, found, err := s.store.LookupUsername(ctx, list[idx].Username)
		if err != nil {
			return nil, err
		}
		if found && id != ch.ID {
			return nil, mtproto.ErrUsernameOccupied
		}
	}
	prev := activeUsernames(list)
	list[idx].Active = active
	f.Usernames = list
	ch.Username = primaryUsername(list)
	ch.Version++
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	s.logUsernames(ctx, ch.ID, self, prev, activeUsernames(list))
	s.channelSettingUpdates(ctx, self, actorAuth, ch.ID)
	return mtproto.BoolTrue, nil
}

// ReorderUsernames sets the order of every active username. The first name
// becomes channel.username.
// channels.reorderUsernames#b45ced1d channel:InputChannel order:Vector<string> = Bool;
func (s *Service) ReorderUsernames(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, order []string) (*mtproto.Bool, error) {
	ch, _, err := s.requireInfoAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	list := append([]UsernameRec(nil), listedUsernames(ch, f)...)
	active := make([]UsernameRec, 0, len(list))
	inactive := make([]UsernameRec, 0)
	for _, u := range list {
		if u.Active {
			active = append(active, u)
		} else {
			inactive = append(inactive, u)
		}
	}
	if len(order) != len(active) {
		return nil, mtproto.ErrUsernameInvalid
	}
	used := make([]bool, len(active))
	reordered := make([]UsernameRec, 0, len(active))
	for _, name := range order {
		found := -1
		for i, u := range active {
			if !used[i] && equalFold(u.Username, name) {
				found = i
				break
			}
		}
		if found < 0 {
			return nil, mtproto.ErrUsernameInvalid
		}
		used[found] = true
		reordered = append(reordered, active[found])
	}
	same := true
	for i := range reordered {
		if !equalFold(reordered[i].Username, active[i].Username) {
			same = false
			break
		}
	}
	if same {
		return nil, mtproto.ErrChatNotModified
	}
	prev := activeUsernames(list)
	next := append(reordered, inactive...)
	f.Usernames = next
	ch.Username = primaryUsername(next)
	ch.Version++
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	s.logUsernames(ctx, ch.ID, self, prev, activeUsernames(next))
	s.channelSettingUpdates(ctx, self, actorAuth, ch.ID)
	return mtproto.BoolTrue, nil
}

// DeactivateAllUsernames turns off every collectible username and leaves the
// editable username untouched.
// channels.deactivateAllUsernames#a245dd3 channel:InputChannel = Bool;
func (s *Service) DeactivateAllUsernames(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel) (*mtproto.Bool, error) {
	ch, _, err := s.requireInfoAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	list := append([]UsernameRec(nil), listedUsernames(ch, f)...)
	prev := activeUsernames(list)
	changed := false
	for i := range list {
		if !list[i].Editable && list[i].Active {
			list[i].Active = false
			changed = true
		}
	}
	if !changed {
		return mtproto.BoolTrue, nil
	}
	f.Usernames = list
	ch.Username = primaryUsername(list)
	ch.Version++
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	if err := s.store.SaveChannel(ctx, ch); err != nil {
		return nil, err
	}
	s.logUsernames(ctx, ch.ID, self, prev, activeUsernames(list))
	s.channelSettingUpdates(ctx, self, actorAuth, ch.ID)
	return mtproto.BoolTrue, nil
}

// ToggleAntiSpam enables the native antispam flag on a supergroup.
// channels.toggleAntiSpam#68f3e4eb channel:InputChannel enabled:Bool = Updates;
func (s *Service) ToggleAntiSpam(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, enabled bool) (*mtproto.Updates, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !ch.Megagroup {
		return nil, mtproto.ErrPeerIdNotSupported
	}
	if !p.IsCreator() && !p.adminRights().CanBanUsers() {
		return nil, mtproto.ErrChatAdminRequired
	}
	f := s.features(ctx, ch.ID)
	if f.Antispam == enabled {
		return nil, mtproto.ErrChatNotModified
	}
	f.Antispam = enabled
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, self, mtproto.MakeTLChannelAdminLogEventActionToggleAntiSpam(&mtproto.ChannelAdminLogEventAction{
		NewValue_BOOL: mtproto.ToBool(enabled),
	}).To_ChannelAdminLogEventAction())
	return s.channelSettingUpdates(ctx, self, actorAuth, ch.ID), nil
}

// ReportAntiSpamFalsePositive records a message the native antispam removed.
// channels.reportAntiSpamFalsePositive#a850a693 channel:InputChannel msg_id:int = Bool;
func (s *Service) ReportAntiSpamFalsePositive(ctx context.Context, self int64, in *mtproto.InputChannel, msgID int32) (*mtproto.Bool, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if msgID <= 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	msg, err := s.store.GetMessage(ctx, ch.ID, msgID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	f := s.features(ctx, ch.ID)
	f.AntispamFalsePositives = append(f.AntispamFalsePositives, FalsePositive{
		ReporterID: self,
		MsgID:      msgID,
		Date:       int32(time.Now().Unix()),
	})
	if len(f.AntispamFalsePositives) > reportCap {
		f.AntispamFalsePositives = f.AntispamFalsePositives[len(f.AntispamFalsePositives)-reportCap:]
	}
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

// SetBoostsToUnblockRestrictions sets how many boosts skip slow mode and
// similar limits. Zero clears the requirement.
// channels.setBoostsToUnblockRestrictions#ad399cee channel:InputChannel boosts:int = Updates;
func (s *Service) SetBoostsToUnblockRestrictions(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, boosts int32) (*mtproto.Updates, error) {
	ch, _, err := s.requireInfoAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if boosts < 0 || boosts > maxBoostsUnrestrict {
		return nil, mtproto.ErrInputRequestInvalid
	}
	f := s.features(ctx, ch.ID)
	if f.BoostsUnrestrict == boosts {
		return nil, mtproto.ErrChatNotModified
	}
	f.BoostsUnrestrict = boosts
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return s.channelSettingUpdates(ctx, self, actorAuth, ch.ID), nil
}

// ToggleAutotranslation sets channel.autotranslation.
// channels.toggleAutotranslation#167fc0a1 channel:InputChannel enabled:Bool = Updates;
func (s *Service) ToggleAutotranslation(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, enabled bool) (*mtproto.Updates, error) {
	ch, _, err := s.requireInfoAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	if f.Autotranslation == enabled {
		return nil, mtproto.ErrChatNotModified
	}
	f.Autotranslation = enabled
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, self, mtproto.MakeTLChannelAdminLogEventActionToggleAutotranslation(&mtproto.ChannelAdminLogEventAction{
		NewValue_BOOL: mtproto.ToBool(enabled),
	}).To_ChannelAdminLogEventAction())
	return s.channelSettingUpdates(ctx, self, actorAuth, ch.ID), nil
}

// UpdatePaidMessagesPrice sets the stars price for sending into the channel.
// channels.updatePaidMessagesPrice#4b12327b flags:# broadcast_messages_allowed:flags.0?true channel:InputChannel send_paid_messages_stars:long = Updates;
func (s *Service) UpdatePaidMessagesPrice(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, stars int64, broadcastAllowed bool) (*mtproto.Updates, error) {
	ch, _, err := s.requireInfoAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if stars < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	f := s.features(ctx, ch.ID)
	if f.PaidMessagesSet && f.SendPaidMessagesStars == stars && f.BroadcastMessagesAllowed == broadcastAllowed {
		return nil, mtproto.ErrChatNotModified
	}
	f.PaidMessagesSet = true
	f.SendPaidMessagesStars = stars
	f.BroadcastMessagesAllowed = broadcastAllowed
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return s.channelSettingUpdates(ctx, self, actorAuth, ch.ID), nil
}

// ToggleViewForumAsMessages stores one user's "view forum as messages" flag.
// channels.toggleViewForumAsMessages#9738bb15 channel:InputChannel enabled:Bool = Updates;
func (s *Service) ToggleViewForumAsMessages(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, enabled bool) (*mtproto.Updates, error) {
	ch, _, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !ch.Forum {
		return nil, mtproto.ErrChannelForumMissing
	}
	f := s.features(ctx, ch.ID)
	if f.ViewForumAsMessages == nil {
		f.ViewForumAsMessages = map[int64]bool{}
	}
	if f.ViewForumAsMessages[self] == enabled {
		return nil, mtproto.ErrChatNotModified
	}
	if enabled {
		f.ViewForumAsMessages[self] = true
	} else {
		delete(f.ViewForumAsMessages, self)
	}
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	upd := mtproto.MakeTLUpdateChannelViewForumAsMessages(&mtproto.Update{
		ChannelId: ch.ID,
		Enabled:   mtproto.ToBool(enabled),
	}).To_Update()
	up := s.pack(ctx, self, ch.ID, upd)
	h := s.hooks()
	if h.Push != nil {
		var excl []int64
		if actorAuth != 0 {
			excl = []int64{actorAuth}
		}
		h.Push(ctx, self, excl, s.pack(ctx, self, ch.ID, upd))
	}
	return up, nil
}

// UpdateColor sets the channel name or profile color.
// channels.updateColor#d8aa3671 flags:# for_profile:flags.1?true channel:InputChannel color:flags.2?int background_emoji_id:flags.0?long = Updates;
func (s *Service) UpdateColor(ctx context.Context, self, actorAuth int64, in *mtproto.TLChannelsUpdateColor) (*mtproto.Updates, error) {
	ch, _, err := s.requireInfoAdmin(ctx, self, in.GetChannel())
	if err != nil {
		return nil, err
	}
	colorID, hasColor, emoji, hasEmoji := selectedChannelColor(in)
	limit := nameColorCount
	if in.GetForProfile() {
		limit = profileColorCount
	}
	if hasColor && (colorID < 0 || int(colorID) >= limit) {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if hasEmoji && emoji < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	f := s.features(ctx, ch.ID)
	prev := currentPeerColor(f, in.GetForProfile())
	if in.GetForProfile() {
		f.ProfileColorSet = hasColor
		if hasColor {
			f.ProfileColorID = colorID
		} else {
			f.ProfileColorID = 0
		}
		if hasEmoji {
			f.ProfileColorEmoji = emoji
		} else {
			f.ProfileColorEmoji = 0
		}
	} else {
		f.ColorSet = hasColor
		if hasColor {
			f.ColorID = colorID
		} else {
			f.ColorID = 0
		}
		if hasEmoji {
			f.ColorEmoji = emoji
		} else {
			f.ColorEmoji = 0
		}
	}
	next := currentPeerColor(f, in.GetForProfile())
	if peerColorSame(prev, next) {
		return nil, mtproto.ErrChatNotModified
	}
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, self, mtproto.MakeTLChannelAdminLogEventActionChangePeerColor(&mtproto.ChannelAdminLogEventAction{
		PrevValue_PEERCOLOR: peerColorOrEmpty(prev),
		NewValue_PEERCOLOR:  peerColorOrEmpty(next),
	}).To_ChannelAdminLogEventAction())
	return s.channelSettingUpdates(ctx, self, actorAuth, ch.ID), nil
}

func selectedChannelColor(in *mtproto.TLChannelsUpdateColor) (id int32, hasColor bool, emoji int64, hasEmoji bool) {
	if in == nil {
		return 0, false, 0, false
	}
	if wrapped := in.GetColor_FLAGINT32(); wrapped != nil {
		id = wrapped.GetValue()
		hasColor = true
	}
	if wrapped := in.GetBackgroundEmojiId(); wrapped != nil {
		emoji = wrapped.GetValue()
		hasEmoji = true
	}
	return id, hasColor, emoji, hasEmoji
}

func currentPeerColor(f *Features, profile bool) *mtproto.PeerColor {
	if f == nil {
		return peerColorValue(false, 0, 0)
	}
	if profile {
		return peerColorValue(f.ProfileColorSet, f.ProfileColorID, f.ProfileColorEmoji)
	}
	return peerColorValue(f.ColorSet, f.ColorID, f.ColorEmoji)
}

func peerColorValue(set bool, id int32, emoji int64) *mtproto.PeerColor {
	if !set && emoji == 0 {
		return nil
	}
	pc := mtproto.MakeTLPeerColor(&mtproto.PeerColor{}).To_PeerColor()
	if set {
		pc.Color = wrapperspb.Int32(id)
	}
	if emoji != 0 {
		pc.BackgroundEmojiId_FLAGINT64 = wrapperspb.Int64(emoji)
	}
	return pc
}

func peerColorOrEmpty(pc *mtproto.PeerColor) *mtproto.PeerColor {
	if pc != nil {
		return pc
	}
	return mtproto.MakeTLPeerColor(&mtproto.PeerColor{}).To_PeerColor()
}

func peerColorSame(a, b *mtproto.PeerColor) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.GetColor().GetValue() == b.GetColor().GetValue() &&
		(a.GetColor() == nil) == (b.GetColor() == nil) &&
		a.GetBackgroundEmojiId_FLAGINT64().GetValue() == b.GetBackgroundEmojiId_FLAGINT64().GetValue()
}

// UpdateEmojiStatus sets or clears the channel emoji status.
// channels.updateEmojiStatus#f0d3e6a8 channel:InputChannel emoji_status:EmojiStatus = Updates;
func (s *Service) UpdateEmojiStatus(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, status *mtproto.EmojiStatus) (*mtproto.Updates, error) {
	ch, _, err := s.requireInfoAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	next, err := storedEmoji(status)
	if err != nil {
		return nil, err
	}
	f := s.features(ctx, ch.ID)
	if emojiSame(f.Emoji, next) {
		return nil, mtproto.ErrChatNotModified
	}
	prev := emojiStatusOrEmpty(f.Emoji)
	f.Emoji = next
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	s.logAdmin(ctx, ch.ID, self, mtproto.MakeTLChannelAdminLogEventActionChangeEmojiStatus(&mtproto.ChannelAdminLogEventAction{
		PrevValue_EMOJISTATUS: prev,
		NewValue_EMOJISTATUS:  emojiStatusOrEmpty(next),
	}).To_ChannelAdminLogEventAction())
	return s.channelSettingUpdates(ctx, self, actorAuth, ch.ID), nil
}

func storedEmoji(status *mtproto.EmojiStatus) (StoredEmojiStatus, error) {
	if status == nil || status.GetPredicateName() == mtproto.Predicate_emojiStatusEmpty ||
		(status.GetPredicateName() == "" && status.GetDocumentId() == 0 && status.GetCollectibleId() == 0) {
		return StoredEmojiStatus{}, nil
	}
	until := status.GetUntil_INT32()
	if status.GetUntil_FLAGINT32() != nil {
		until = status.GetUntil_FLAGINT32().GetValue()
	}
	switch status.GetPredicateName() {
	case mtproto.Predicate_emojiStatusCollectible, mtproto.Predicate_inputEmojiStatusCollectible:
		if status.GetCollectibleId() == 0 {
			return StoredEmojiStatus{}, mtproto.ErrInputRequestInvalid
		}
		return StoredEmojiStatus{
			Collectible:       true,
			CollectibleID:     status.GetCollectibleId(),
			DocumentID:        status.GetDocumentId(),
			Until:             until,
			Title:             status.GetTitle(),
			Slug:              status.GetSlug(),
			PatternDocumentID: status.GetPatternDocumentId(),
			CenterColor:       status.GetCenterColor(),
			EdgeColor:         status.GetEdgeColor(),
			PatternColor:      status.GetPatternColor(),
			TextColor:         status.GetTextColor(),
		}, nil
	default:
		if status.GetDocumentId() == 0 {
			return StoredEmojiStatus{}, nil
		}
		if status.GetDocumentId() < 0 {
			return StoredEmojiStatus{}, mtproto.ErrInputRequestInvalid
		}
		return StoredEmojiStatus{DocumentID: status.GetDocumentId(), Until: until}, nil
	}
}

func emojiSame(a, b StoredEmojiStatus) bool {
	return a == b
}

func emojiStatusValue(e StoredEmojiStatus) *mtproto.EmojiStatus {
	if e.unset() {
		return nil
	}
	if e.Collectible {
		st := mtproto.MakeTLEmojiStatusCollectible(&mtproto.EmojiStatus{
			CollectibleId:     e.CollectibleID,
			DocumentId:        e.DocumentID,
			Title:             e.Title,
			Slug:              e.Slug,
			PatternDocumentId: e.PatternDocumentID,
			CenterColor:       e.CenterColor,
			EdgeColor:         e.EdgeColor,
			PatternColor:      e.PatternColor,
			TextColor:         e.TextColor,
			Until_INT32:       e.Until,
		}).To_EmojiStatus()
		if e.Until != 0 {
			st.Until_FLAGINT32 = wrapperspb.Int32(e.Until)
		}
		return st
	}
	st := mtproto.MakeTLEmojiStatus(&mtproto.EmojiStatus{
		DocumentId:  e.DocumentID,
		Until_INT32: e.Until,
	}).To_EmojiStatus()
	if e.Until != 0 {
		st.Until_FLAGINT32 = wrapperspb.Int32(e.Until)
	}
	return st
}

func emojiStatusOrEmpty(e StoredEmojiStatus) *mtproto.EmojiStatus {
	if st := emojiStatusValue(e); st != nil {
		return st
	}
	return mtproto.MakeTLEmojiStatusEmpty(&mtproto.EmojiStatus{}).To_EmojiStatus()
}

func paidStars(f *Features) *wrapperspb.Int64Value {
	if f == nil || !f.PaidMessagesSet {
		return nil
	}
	return wrapperspb.Int64(f.SendPaidMessagesStars)
}

// SetMainProfileTab selects the default profile tab.
// channels.setMainProfileTab#3583fcb1 channel:InputChannel tab:ProfileTab = Bool;
func (s *Service) SetMainProfileTab(ctx context.Context, self, actorAuth int64, in *mtproto.InputChannel, tab *mtproto.ProfileTab) (*mtproto.Bool, error) {
	ch, _, err := s.requireInfoAdmin(ctx, self, in)
	if err != nil {
		return nil, err
	}
	kind := mtproto.FromProfileTabToType(tab)
	if kind == mtproto.ProfileTabTypeUnknown {
		return nil, mtproto.ErrInputRequestInvalid
	}
	f := s.features(ctx, ch.ID)
	if f.MainProfileTab == kind {
		return mtproto.BoolTrue, nil
	}
	f.MainProfileTab = kind
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	s.channelSettingUpdates(ctx, self, actorAuth, ch.ID)
	return mtproto.BoolTrue, nil
}

// GetChannelRecommendations returns other public channels, largest first.
// The requested channel and channels the user already belongs to are omitted.
// channels.getChannelRecommendations#25a71742 flags:# channel:flags.0?InputChannel = messages.Chats;
func (s *Service) GetChannelRecommendations(ctx context.Context, self int64, in *mtproto.InputChannel) (*mtproto.Messages_Chats, error) {
	var skip int64
	if in != nil && in.GetPredicateName() != "" && in.GetPredicateName() != mtproto.Predicate_inputChannelEmpty {
		ch, _, err := s.requireView(ctx, self, in)
		if err != nil {
			return nil, err
		}
		skip = ch.ID
	}
	joined, err := s.store.ListByUser(ctx, self)
	if err != nil {
		return nil, err
	}
	member := map[int64]bool{}
	for _, ch := range joined {
		member[ch.ID] = true
	}
	public, err := s.store.ListPublic(ctx)
	if err != nil {
		return nil, err
	}
	matched := make([]*Channel, 0)
	for _, ch := range public {
		if ch.ID == skip || member[ch.ID] {
			continue
		}
		matched = append(matched, ch)
	}
	page := matched
	if len(page) > recommendationCap {
		page = page[:recommendationCap]
	}
	chats := make([]*mtproto.Chat, 0, len(page))
	for _, ch := range page {
		chats = append(chats, s.ChatFor(ctx, self, ch))
	}
	if len(matched) > len(page) {
		return mtproto.MakeTLMessagesChatsSlice(&mtproto.Messages_Chats{
			Count: int32(len(matched)),
			Chats: chats,
		}).To_Messages_Chats(), nil
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
}

// GetLeftChannels lists channels the user left, offset channels skipped.
// channels.getLeftChannels#8341ecc0 offset:int = messages.Chats;
func (s *Service) GetLeftChannels(ctx context.Context, self int64, offset int32) (*mtproto.Messages_Chats, error) {
	if offset < 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	all, err := s.store.ListLeftByUser(ctx, self)
	if err != nil {
		return nil, err
	}
	if int(offset) > len(all) {
		offset = int32(len(all))
	}
	rest := all[offset:]
	page := rest
	if len(page) > leftChannelsPage {
		page = page[:leftChannelsPage]
	}
	chats := make([]*mtproto.Chat, 0, len(page))
	for _, ch := range page {
		p, _ := s.store.GetParticipant(ctx, ch.ID, self)
		chats = append(chats, s.chatFor(ctx, self, ch, p))
	}
	if len(rest) > len(page) {
		return mtproto.MakeTLMessagesChatsSlice(&mtproto.Messages_Chats{
			Count: int32(len(all)),
			Chats: chats,
		}).To_Messages_Chats(), nil
	}
	return mtproto.MakeTLMessagesChats(&mtproto.Messages_Chats{Chats: chats}).To_Messages_Chats(), nil
}

// ReportSpam records messages from a participant as spam. The caller must be
// an administrator.
// channels.reportSpam#f44a8315 channel:InputChannel participant:InputPeer id:Vector<int> = Bool;
func (s *Service) ReportSpam(ctx context.Context, self int64, in *mtproto.InputChannel, participant *mtproto.InputPeer, ids []int32) (*mtproto.Bool, error) {
	ch, p, err := s.requireMember(ctx, self, in)
	if err != nil {
		return nil, err
	}
	if !p.IsAdmin() {
		return nil, mtproto.ErrChatAdminRequired
	}
	if len(ids) == 0 {
		return nil, mtproto.ErrMessageIdsEmpty
	}
	userID, err := reportedUserID(participant)
	if err != nil {
		return nil, err
	}
	target, err := s.store.GetParticipant(ctx, ch.ID, userID)
	if err != nil {
		return nil, err
	}
	if target == nil {
		return nil, mtproto.ErrUserNotParticipant
	}
	for _, id := range ids {
		if id <= 0 {
			return nil, mtproto.ErrMessageIdInvalid
		}
		msg, err := s.store.GetMessage(ctx, ch.ID, id)
		if err != nil {
			return nil, err
		}
		if msg == nil {
			return nil, mtproto.ErrMessageIdInvalid
		}
	}
	f := s.features(ctx, ch.ID)
	copied := append([]int32(nil), ids...)
	f.SpamReports = append(f.SpamReports, SpamReport{
		ReporterID: self,
		UserID:     userID,
		MsgIDs:     copied,
		Date:       int32(time.Now().Unix()),
	})
	if len(f.SpamReports) > reportCap {
		f.SpamReports = f.SpamReports[len(f.SpamReports)-reportCap:]
	}
	if err := s.saveFeatures(ctx, ch.ID, f); err != nil {
		return nil, err
	}
	return mtproto.BoolTrue, nil
}

func reportedUserID(peer *mtproto.InputPeer) (int64, error) {
	if peer == nil || peer.GetPredicateName() == "" || peer.GetPredicateName() == mtproto.Predicate_inputPeerEmpty {
		return 0, mtproto.ErrPeerIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerUser, mtproto.Predicate_inputPeerUserFromMessage:
		if peer.GetUserId() == 0 {
			return 0, mtproto.ErrPeerIdInvalid
		}
		return peer.GetUserId(), nil
	default:
		return 0, mtproto.ErrPeerIdInvalid
	}
}

// RecordPeerReport stores a peer report that is not scoped to one channel.
func (s *Service) RecordPeerReport(ctx context.Context, self int64, peer *mtproto.InputPeer, kind, message string, photoID int64) (*mtproto.Bool, error) {
	id, err := reportPeerID(ctx, s, self, peer)
	if err != nil {
		return nil, err
	}
	s.reportMu.Lock()
	s.peerReports = append(s.peerReports, PeerReport{
		ReporterID: self,
		PeerID:     id,
		Kind:       kind,
		Message:    message,
		PhotoID:    photoID,
		Date:       int32(time.Now().Unix()),
	})
	if len(s.peerReports) > reportCap {
		s.peerReports = s.peerReports[len(s.peerReports)-reportCap:]
	}
	s.reportMu.Unlock()
	return mtproto.BoolTrue, nil
}

// PeerReports returns recorded peer reports. Tests use it.
func (s *Service) PeerReports() []PeerReport {
	s.reportMu.Lock()
	defer s.reportMu.Unlock()
	out := make([]PeerReport, len(s.peerReports))
	copy(out, s.peerReports)
	return out
}

func reportPeerID(ctx context.Context, s *Service, self int64, peer *mtproto.InputPeer) (int64, error) {
	if peer == nil || peer.GetPredicateName() == "" || peer.GetPredicateName() == mtproto.Predicate_inputPeerEmpty {
		return 0, mtproto.ErrPeerIdInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		if self == 0 {
			return 0, mtproto.ErrPeerIdInvalid
		}
		return self, nil
	case mtproto.Predicate_inputPeerUser, mtproto.Predicate_inputPeerUserFromMessage:
		if peer.GetUserId() == 0 {
			return 0, mtproto.ErrPeerIdInvalid
		}
		return peer.GetUserId(), nil
	case mtproto.Predicate_inputPeerChat:
		if peer.GetChatId() == 0 {
			return 0, mtproto.ErrChatIdInvalid
		}
		return peer.GetChatId(), nil
	case mtproto.Predicate_inputPeerChannel, mtproto.Predicate_inputPeerChannelFromMessage:
		if peer.GetChannelId() == 0 {
			return 0, mtproto.ErrChannelInvalid
		}
		ch, err := s.store.GetChannel(ctx, peer.GetChannelId())
		if err != nil {
			return 0, err
		}
		if ch == nil || ch.Deleted {
			return 0, mtproto.ErrChannelInvalid
		}
		return ch.ID, nil
	default:
		return 0, mtproto.ErrPeerIdInvalid
	}
}
