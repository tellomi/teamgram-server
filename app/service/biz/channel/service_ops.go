package channel

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func (s *Service) lookup(ctx context.Context, in *mtproto.InputChannel) (*Channel, error) {
	if in == nil || in.PredicateName == mtproto.Predicate_inputChannelEmpty || in.ChannelId == 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	ch, err := s.store.GetChannel(ctx, in.ChannelId)
	if err != nil {
		return nil, err
	}
	if ch == nil || ch.Deleted {
		return nil, mtproto.ErrChannelInvalid
	}
	if in.AccessHash != 0 && in.AccessHash != ch.AccessHash {
		return nil, mtproto.ErrChannelInvalid
	}
	return ch, nil
}

func (s *Service) reload(ctx context.Context, id int64) (*Channel, error) {
	ch, err := s.store.GetChannel(ctx, id)
	if err != nil {
		return nil, err
	}
	if ch == nil || ch.Deleted {
		return nil, mtproto.ErrChannelInvalid
	}
	return ch, nil
}

func (s *Service) requireView(ctx context.Context, userID int64, in *mtproto.InputChannel) (*Channel, *Participant, error) {
	ch, err := s.lookup(ctx, in)
	if err != nil {
		return nil, nil, err
	}
	p, err := s.store.GetParticipant(ctx, ch.ID, userID)
	if err != nil {
		return nil, nil, err
	}
	if p != nil && p.InChannel() {
		if blocked, _ := s.viewBlocked(p); blocked {
			return nil, p, mtproto.ErrChannelPrivate
		}
		return ch, p, nil
	}
	if ch.Public() {
		return ch, p, nil
	}
	return nil, p, mtproto.ErrChannelPrivate
}

func (s *Service) requireMember(ctx context.Context, userID int64, in *mtproto.InputChannel) (*Channel, *Participant, error) {
	ch, err := s.lookup(ctx, in)
	if err != nil {
		return nil, nil, err
	}
	p, err := s.store.GetParticipant(ctx, ch.ID, userID)
	if err != nil {
		return nil, nil, err
	}
	if p == nil || !p.InChannel() {
		return nil, p, mtproto.ErrChannelPrivate
	}
	if blocked, _ := s.viewBlocked(p); blocked {
		return nil, p, mtproto.ErrUserBannedInChannel
	}
	return ch, p, nil
}

func (s *Service) viewBlocked(p *Participant) (bool, *mtproto.ChatBannedRights) {
	if p == nil || p.BannedRights == 0 {
		return false, nil
	}
	br := mtproto.BannedRights(p.BannedRights).ToChatBannedRights()
	if br == nil {
		return false, nil
	}
	return !br.CanViewMessages(int32(time.Now().Unix())), br
}

func (s *Service) canInvite(ch *Channel, p *Participant) bool {
	if p == nil || !p.InChannel() {
		return false
	}
	if p.IsCreator() || p.adminRights().CanInviteUsers() {
		return true
	}
	if ch.Broadcast {
		return false
	}
	return s.defaultAllows(ch, func(br *mtproto.ChatBannedRights) bool {
		return br.CanInviteUsers(int32(time.Now().Unix()))
	})
}

func (s *Service) canChangeInfo(ch *Channel, p *Participant) bool {
	if p == nil || !p.InChannel() {
		return false
	}
	if p.IsCreator() || p.adminRights().CanChangeInfo() {
		return true
	}
	if ch.Broadcast {
		return false
	}
	return s.defaultAllows(ch, func(br *mtproto.ChatBannedRights) bool {
		return br.CanChangeInfo(int32(time.Now().Unix()))
	})
}

func (s *Service) defaultAllows(ch *Channel, allow func(*mtproto.ChatBannedRights) bool) bool {
	if ch.DefaultBanned == 0 {
		return true
	}
	br := mtproto.BannedRights(ch.DefaultBanned).ToChatBannedRights()
	if br == nil {
		return true
	}
	return allow(br)
}

func (s *Service) canPost(ch *Channel, p *Participant) error {
	if p == nil || !p.InChannel() {
		return mtproto.ErrChannelPrivate
	}
	now := int32(time.Now().Unix())
	if blocked, br := s.viewBlocked(p); blocked {
		return mtproto.ErrUserBannedInChannel
	} else if br != nil && !br.CanSendMessages(now) && !p.IsAdmin() {
		return mtproto.ErrChatWriteForbidden
	}
	if p.IsCreator() {
		return nil
	}
	if ch.Broadcast {
		if p.IsAdmin() && p.adminRights().CanPostMessages() {
			return nil
		}
		return mtproto.ErrChatWriteForbidden
	}
	if p.IsAdmin() {
		return nil
	}
	if p.BannedRights != 0 {
		br := mtproto.BannedRights(p.BannedRights).ToChatBannedRights()
		if br != nil && !br.CanSendMessages(now) {
			return mtproto.ErrChatWriteForbidden
		}
	}
	if !s.defaultAllows(ch, func(br *mtproto.ChatBannedRights) bool { return br.CanSendMessages(now) }) {
		return mtproto.ErrChatWriteForbidden
	}
	return nil
}

func (s *Service) refreshCount(ctx context.Context, channelID int64) (*Channel, error) {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return nil, err
	}
	n, err := s.store.CountMembers(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if ch.ParticipantsCount != n {
		ch.ParticipantsCount = n
		ch.Version++
		if err = s.store.SaveChannel(ctx, ch); err != nil {
			return nil, err
		}
	}
	return s.reload(ctx, channelID)
}

func (s *Service) participantCounts(ctx context.Context, channelID int64) (admins, kicked, banned int32, err error) {
	all, err := s.store.ListParticipants(ctx, channelID)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, p := range all {
		if p.IsAdmin() {
			admins++
		}
		if p.State == StateKicked {
			kicked++
		}
		if p.Role == RoleBanned || p.State == StateKicked {
			banned++
		}
	}
	return admins, kicked, banned, nil
}

func serviceMessage(sender, channelID int64, action *mtproto.MessageAction) *mtproto.Message {
	return mtproto.MakeTLMessageService(&mtproto.Message{
		FromId: mtproto.MakePeerUser(sender),
		PeerId: mtproto.MakePeerChannel(channelID),
		Date:   int32(time.Now().Unix()),
		Action: action,
	}).To_Message()
}

func (s *Service) appendMessage(ctx context.Context, channelID, sender, randomID int64, msg *mtproto.Message) (int32, int32, error) {
	if msg.Date == 0 {
		msg.Date = int32(time.Now().Unix())
	}
	if msg != nil {
		msg.SanitizeEntities()
	}
	var stored *mtproto.Message
	id, pts, reused, err := s.store.CommitNew(ctx, channelID, sender, randomID, msg.Date, func(messageID, pts int32) []byte {
		m := proto.Clone(msg).(*mtproto.Message)
		m.Id = messageID
		m.Out = false
		m.PeerId = mtproto.MakePeerChannel(channelID)
		m.SanitizeEntities()
		stored = m
		b, _ := proto.Marshal(m)
		return b
	})
	if err != nil {
		return 0, 0, err
	}
	if reused {
		return id, pts, nil
	}
	_ = stored
	s.noteNewMessage(ctx, channelID, sender, id)
	return id, pts, nil
}

func (s *Service) noteNewMessage(ctx context.Context, channelID, sender int64, messageID int32) {
	parts, err := s.store.ListParticipants(ctx, channelID)
	if err != nil {
		return
	}
	for _, p := range parts {
		if p.State != StateNormal {
			continue
		}
		st, _ := s.store.GetRead(ctx, channelID, p.UserID)
		if st == nil {
			st = &ReadState{ChannelID: channelID, UserID: p.UserID}
		}
		if p.UserID == sender {
			// The author has seen their own message. That is the inbox cursor.
			// read_outbox_max_id moves only when a different member reads.
			if messageID > st.ReadInboxMaxID {
				st.ReadInboxMaxID = messageID
			}
			st.UnreadCount = 0
		} else if messageID > st.ReadInboxMaxID {
			st.UnreadCount++
		}
		_ = s.store.SaveRead(ctx, st)
	}
}

func (s *Service) prepareOutgoing(ch *Channel, sender int64, msg *mtproto.Message) {
	msg.PeerId = mtproto.MakePeerChannel(ch.ID)
	msg.Out = false
	if msg.PredicateName == mtproto.Predicate_messageService {
		msg.Post = false
		if msg.FromId == nil {
			msg.FromId = mtproto.MakePeerUser(sender)
		}
		return
	}
	if ch.Broadcast {
		msg.Post = true
		if ch.Signatures {
			msg.FromId = mtproto.MakePeerUser(sender)
		} else {
			msg.FromId = nil
		}
		if msg.Views == nil {
			msg.Views = &wrapperspb.Int32Value{Value: 1}
		}
		return
	}
	msg.Post = false
	msg.FromId = mtproto.MakePeerUser(sender)
	msg.Views = nil
}

// Outgoing is one message a client is sending into a channel.
type Outgoing struct {
	RandomID int64
	Message  *mtproto.Message
}

func (s *Service) Send(ctx context.Context, sender, channelID, actorAuth int64, batch []Outgoing) (*mtproto.Updates, error) {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return nil, err
	}
	p, err := s.store.GetParticipant(ctx, channelID, sender)
	if err != nil {
		return nil, err
	}
	if err = s.canPost(ch, p); err != nil {
		return nil, err
	}
	feat := s.features(ctx, channelID)
	if feat.SlowmodeSeconds > 0 && p != nil && !p.IsAdmin() {
		if len(batch) > 1 {
			return nil, mtproto.ErrSlowmodeMultiMsgsDisabled
		}
		now := int32(time.Now().Unix())
		if last := feat.LastSend[sender]; last != 0 && now-last < feat.SlowmodeSeconds {
			return nil, mtproto.NewErrSlowModeWaitX(feat.SlowmodeSeconds - (now - last))
		}
		feat.LastSend[sender] = now
		if err := s.saveFeatures(ctx, channelID, feat); err != nil {
			return nil, err
		}
	}
	if len(batch) == 0 {
		return nil, mtproto.ErrMessageEmpty
	}
	var ups []*mtproto.Update
	for _, item := range batch {
		if item.Message == nil {
			return nil, mtproto.ErrMessageEmpty
		}
		msg := proto.Clone(item.Message).(*mtproto.Message)
		s.prepareOutgoing(ch, sender, msg)
		id, pts, err := s.appendMessage(ctx, channelID, sender, item.RandomID, msg)
		if err != nil {
			return nil, err
		}
		stored, err := s.store.GetMessage(ctx, channelID, id)
		if err != nil || stored == nil {
			return nil, mtproto.ErrInternalServerError
		}
		body, err := unmarshalMessage(stored.Data)
		if err != nil {
			return nil, err
		}
		ups = append(ups, newMessageUpdate(viewMessage(body, stored.SenderID, sender), pts, 1))
	}
	ch, _ = s.reload(ctx, channelID)
	updates := s.updatesWith(ctx, sender, ch, ups...)
	s.emit(ctx, sender, actorAuth, channelID, func(viewer int64) *mtproto.Updates {
		cloned := make([]*mtproto.Update, 0, len(ups))
		for _, u := range ups {
			cu := proto.Clone(u).(*mtproto.Update)
			if cu.Message_MESSAGE != nil {
				cu.Message_MESSAGE = viewMessage(cu.Message_MESSAGE, sender, viewer)
			}
			cloned = append(cloned, cu)
		}
		return s.updatesWith(ctx, viewer, ch, cloned...)
	})
	return updates, nil
}

func (s *Service) Edit(ctx context.Context, sender, channelID, actorAuth int64, messageID int32, apply func(*mtproto.Message)) (*mtproto.Updates, error) {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return nil, err
	}
	p, err := s.store.GetParticipant(ctx, channelID, sender)
	if err != nil {
		return nil, err
	}
	stored, err := s.store.GetMessage(ctx, channelID, messageID)
	if err != nil {
		return nil, err
	}
	if stored == nil || stored.Deleted {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if err = s.canModify(ch, p, stored, true); err != nil {
		return nil, err
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return nil, err
	}
	if apply != nil {
		apply(body)
	}
	body.Id = messageID
	body.EditDate = &wrapperspb.Int32Value{Value: int32(time.Now().Unix())}
	s.prepareOutgoing(ch, stored.SenderID, body)
	body.SanitizeEntities()
	raw, err := proto.Marshal(body)
	if err != nil {
		return nil, err
	}
	pts, err := s.store.CommitEdit(ctx, channelID, messageID, body.EditDate.Value, raw)
	if err != nil {
		return nil, err
	}
	upd := editMessageUpdate(viewMessage(body, stored.SenderID, sender), pts, 1)
	updates := s.updatesWith(ctx, sender, ch, upd)
	s.emit(ctx, sender, actorAuth, channelID, func(viewer int64) *mtproto.Updates {
		return s.updatesWith(ctx, viewer, ch, editMessageUpdate(viewMessage(body, stored.SenderID, viewer), pts, 1))
	})
	return updates, nil
}

func (s *Service) DeleteInput(ctx context.Context, sender, actorAuth int64, in *mtproto.InputChannel, ids []int32) (int32, int32, error) {
	ch, _, err := s.requireMember(ctx, sender, in)
	if err != nil {
		return 0, 0, err
	}
	return s.Delete(ctx, sender, ch.ID, actorAuth, ids)
}

func (s *Service) Delete(ctx context.Context, sender, channelID, actorAuth int64, ids []int32) (int32, int32, error) {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return 0, 0, err
	}
	p, err := s.store.GetParticipant(ctx, channelID, sender)
	if err != nil {
		return 0, 0, err
	}
	var allowed []int32
	for _, id := range ids {
		stored, err := s.store.GetMessage(ctx, channelID, id)
		if err != nil || stored == nil || stored.Deleted {
			continue
		}
		if s.canModify(ch, p, stored, false) != nil {
			continue
		}
		allowed = append(allowed, id)
	}
	pts, count, deleted, err := s.store.CommitDelete(ctx, channelID, allowed)
	if err != nil {
		return 0, 0, err
	}
	if count > 0 {
		s.emit(ctx, sender, actorAuth, channelID, func(viewer int64) *mtproto.Updates {
			return s.updatesWith(ctx, viewer, ch, deleteUpdate(channelID, deleted, pts, count))
		})
	}
	return pts, count, nil
}

func (s *Service) canModify(ch *Channel, actor *Participant, msg *StoredMessage, edit bool) error {
	if actor == nil || !actor.InChannel() {
		return mtproto.ErrChannelPrivate
	}
	if actor.IsCreator() {
		return nil
	}
	if edit && actor.IsAdmin() && actor.adminRights().CanEditMessages() {
		return nil
	}
	if !edit && actor.IsAdmin() && actor.adminRights().CanDeleteMessages() {
		return nil
	}
	if ch.Broadcast {
		return mtproto.ErrMessageAuthorRequired
	}
	if msg.SenderID == actor.UserID {
		return nil
	}
	return mtproto.ErrMessageAuthorRequired
}

func (s *Service) ReadHistory(ctx context.Context, user int64, in *mtproto.InputChannel, maxID int32) error {
	ch, _, err := s.requireMember(ctx, user, in)
	if err != nil {
		return err
	}
	if maxID <= 0 || maxID > ch.TopMessage {
		maxID = ch.TopMessage
	}
	st, err := s.store.GetRead(ctx, ch.ID, user)
	if err != nil {
		return err
	}
	if maxID > st.ReadInboxMaxID {
		st.ReadInboxMaxID = maxID
	}
	msgs, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return err
	}
	var unread int32
	for _, m := range msgs {
		if !m.Deleted && m.ID > st.ReadInboxMaxID {
			unread++
		}
	}
	st.UnreadCount = unread
	if err = s.store.SaveRead(ctx, st); err != nil {
		return err
	}
	// The reader's own outbox stays put. Every other member's outbox moves
	// to this id, which is "any other member has read up to here".
	advanced, err := s.advanceOthersOutbox(ctx, ch.ID, user, maxID)
	if err != nil {
		return err
	}
	s.pushOutboxRead(ctx, ch.ID, maxID, advanced)
	s.syncDialogRead(ctx, ch.ID, user)
	for _, id := range advanced {
		s.syncDialogRead(ctx, ch.ID, id)
	}
	return nil
}

// syncDialogRead copies one member's channel read cursors onto their dialog
// row. messages.getPeerDialogs reads that row.
func (s *Service) syncDialogRead(ctx context.Context, channelID, userID int64) {
	h := s.hooks()
	if h.Dialog == nil || userID == 0 {
		return
	}
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return
	}
	st, err := s.store.GetRead(ctx, channelID, userID)
	if err != nil || st == nil {
		return
	}
	date := int64(ch.Date)
	if msg, _ := s.store.GetMessage(ctx, channelID, ch.TopMessage); msg != nil {
		date = int64(msg.Date)
	}
	h.Dialog(ctx, userID, channelID, ch.TopMessage, st.ReadInboxMaxID, st.ReadOutboxMaxID, st.UnreadCount, date)
}

// advanceOthersOutbox moves every other member's outbox up to maxID.
// The member who just read is skipped: their own read is not a receipt.
func (s *Service) advanceOthersOutbox(ctx context.Context, channelID, reader int64, maxID int32) ([]int64, error) {
	if reader == 0 || maxID <= 0 {
		return nil, nil
	}
	parts, err := s.store.ListParticipants(ctx, channelID)
	if err != nil {
		return nil, err
	}
	var advanced []int64
	for _, p := range parts {
		if p.State != StateNormal || p.UserID == 0 || p.UserID == reader {
			continue
		}
		st, err := s.store.GetRead(ctx, channelID, p.UserID)
		if err != nil {
			return nil, err
		}
		if st == nil {
			st = &ReadState{ChannelID: channelID, UserID: p.UserID}
		}
		if maxID <= st.ReadOutboxMaxID {
			continue
		}
		st.ReadOutboxMaxID = maxID
		if err := s.store.SaveRead(ctx, st); err != nil {
			return nil, err
		}
		advanced = append(advanced, p.UserID)
	}
	return advanced, nil
}

func (s *Service) pushOutboxRead(ctx context.Context, channelID int64, maxID int32, users []int64) {
	h := s.hooks()
	if h.Push == nil || len(users) == 0 {
		return
	}
	for _, id := range users {
		upd := mtproto.MakeTLUpdateReadChannelOutbox(&mtproto.Update{
			ChannelId: channelID,
			MaxId:     maxID,
		}).To_Update()
		h.Push(ctx, id, nil, s.pack(ctx, id, channelID, upd))
	}
}

func (s *Service) History(ctx context.Context, user int64, in *mtproto.InputChannel, q HistoryQuery) (*mtproto.Messages_Messages, error) {
	ch, _, err := s.requireView(ctx, user, in)
	if err != nil {
		return nil, err
	}
	return s.history(ctx, user, ch, q)
}

func (s *Service) HistoryByID(ctx context.Context, user, channelID int64, q HistoryQuery) (*mtproto.Messages_Messages, error) {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if _, _, err = s.requireView(ctx, user, &mtproto.InputChannel{
		PredicateName: mtproto.Predicate_inputChannel,
		ChannelId:     ch.ID,
		AccessHash:    ch.AccessHash,
	}); err != nil {
		return nil, err
	}
	return s.history(ctx, user, ch, q)
}

func (s *Service) history(ctx context.Context, user int64, ch *Channel, q HistoryQuery) (*mtproto.Messages_Messages, error) {
	all, err := s.store.ListMessages(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	page, total := selectHistory(all, q)
	feat := s.features(ctx, ch.ID)
	viewer, _ := s.store.GetParticipant(ctx, ch.ID, user)
	minID := feat.AvailableMinID
	if feat.UserMinID != nil && feat.UserMinID[user] > minID {
		minID = feat.UserMinID[user]
	}
	filtered := page[:0]
	for _, stored := range page {
		if stored.ID <= minID {
			continue
		}
		if feat.HiddenPrehistory && (viewer == nil || !viewer.IsAdmin()) && viewer != nil && stored.Date < viewer.Date {
			continue
		}
		filtered = append(filtered, stored)
	}
	page = filtered
	msgs := make([]*mtproto.Message, 0, len(page))
	for _, stored := range page {
		body, err := unmarshalMessage(stored.Data)
		if err != nil {
			continue
		}
		if stored.EditDate != 0 && body.EditDate == nil {
			body.EditDate = &wrapperspb.Int32Value{Value: stored.EditDate}
		}
		msgs = append(msgs, viewMessage(body, stored.SenderID, user))
	}
	return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
		Pts:      ch.Pts,
		Count:    total,
		Messages: msgs,
		Chats:    []*mtproto.Chat{s.ChatFor(ctx, user, ch)},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

func (s *Service) GetMessages(ctx context.Context, user int64, in *mtproto.InputChannel, ids []int32) (*mtproto.Messages_Messages, error) {
	ch, _, err := s.requireView(ctx, user, in)
	if err != nil {
		return nil, err
	}
	msgs := make([]*mtproto.Message, 0, len(ids))
	for _, id := range ids {
		stored, err := s.store.GetMessage(ctx, ch.ID, id)
		if err != nil {
			return nil, err
		}
		if stored == nil || stored.Deleted {
			msgs = append(msgs, mtproto.MakeTLMessageEmpty(&mtproto.Message{
				Id:     id,
				PeerId: mtproto.MakePeerChannel(ch.ID),
			}).To_Message())
			continue
		}
		body, err := unmarshalMessage(stored.Data)
		if err != nil {
			return nil, err
		}
		msgs = append(msgs, viewMessage(body, stored.SenderID, user))
	}
	return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
		Pts:      ch.Pts,
		Count:    int32(len(msgs)),
		Messages: msgs,
		Chats:    []*mtproto.Chat{s.ChatFor(ctx, user, ch)},
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), nil
}

func (s *Service) Difference(ctx context.Context, user int64, in *mtproto.InputChannel, pts, limit int32) (*mtproto.Updates_ChannelDifference, error) {
	ch, _, err := s.requireView(ctx, user, in)
	if err != nil {
		return nil, err
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	timeout := &wrapperspb.Int32Value{Value: 30}
	if pts < 0 {
		return nil, mtproto.ErrChannelInvalid
	}
	if pts >= ch.Pts {
		return mtproto.MakeTLUpdatesChannelDifferenceEmpty(&mtproto.Updates_ChannelDifference{
			Final:   true,
			Pts:     ch.Pts,
			Timeout: timeout,
		}).To_Updates_ChannelDifference(), nil
	}
	events, err := s.store.ListPtsAfter(ctx, ch.ID, pts, 0)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 || events[0].Pts-events[0].PtsCount > pts {
		return s.tooLong(ctx, user, ch, timeout), nil
	}
	if len(events) > 500 && int(limit) < len(events) {
		return s.tooLong(ctx, user, ch, timeout), nil
	}
	final := true
	if len(events) > int(limit) {
		events = events[:limit]
		final = false
	}
	var newMessages []*mtproto.Message
	var other []*mtproto.Update
	endPts := pts
	for _, e := range events {
		endPts = e.Pts
		switch e.Kind {
		case KindNew:
			body, err := unmarshalMessage(e.Payload)
			if err != nil {
				continue
			}
			view := viewMessage(body, s.eventSender(ctx, ch.ID, e, body), user)
			newMessages = append(newMessages, view)
			other = append(other, newMessageUpdate(view, e.Pts, e.PtsCount))
		case KindEdit:
			body, err := unmarshalMessage(e.Payload)
			if err != nil {
				continue
			}
			other = append(other, editMessageUpdate(viewMessage(body, s.eventSender(ctx, ch.ID, e, body), user), e.Pts, e.PtsCount))
		case KindDelete:
			other = append(other, deleteUpdate(ch.ID, e.MessageIDs, e.Pts, e.PtsCount))
		}
	}
	if newMessages == nil {
		newMessages = []*mtproto.Message{}
	}
	if other == nil {
		other = []*mtproto.Update{}
	}
	return mtproto.MakeTLUpdatesChannelDifference(&mtproto.Updates_ChannelDifference{
		Final:        final,
		Pts:          endPts,
		Timeout:      timeout,
		NewMessages:  newMessages,
		OtherUpdates: other,
		Chats:        []*mtproto.Chat{s.ChatFor(ctx, user, ch)},
		Users:        []*mtproto.User{},
	}).To_Updates_ChannelDifference(), nil
}

func (s *Service) eventSender(ctx context.Context, channelID int64, e *PtsEvent, body *mtproto.Message) int64 {
	if e.MessageID != 0 {
		if stored, err := s.store.GetMessage(ctx, channelID, e.MessageID); err == nil && stored != nil && stored.SenderID != 0 {
			return stored.SenderID
		}
	}
	return senderOf(body)
}

func senderOf(m *mtproto.Message) int64 {
	if m == nil || m.FromId == nil {
		return 0
	}
	return m.FromId.GetUserId()
}

func (s *Service) tooLong(ctx context.Context, user int64, ch *Channel, timeout *wrapperspb.Int32Value) *mtproto.Updates_ChannelDifference {
	st, _ := s.store.GetRead(ctx, ch.ID, user)
	if st == nil {
		st = &ReadState{}
	}
	page, _ := selectHistory(mustMessages(s, ctx, ch.ID), HistoryQuery{Limit: 10})
	msgs := make([]*mtproto.Message, 0, len(page))
	for _, stored := range page {
		body, err := unmarshalMessage(stored.Data)
		if err == nil {
			msgs = append(msgs, viewMessage(body, stored.SenderID, user))
		}
	}
	dlg := mtproto.MakeTLDialog(&mtproto.Dialog{
		Peer:                mtproto.MakePeerChannel(ch.ID),
		TopMessage:          ch.TopMessage,
		ReadInboxMaxId:      st.ReadInboxMaxID,
		ReadOutboxMaxId:     st.ReadOutboxMaxID,
		UnreadCount:         st.UnreadCount,
		NotifySettings:      defaultNotify(),
		Pts:                 &wrapperspb.Int32Value{Value: ch.Pts},
		ViewForumAsMessages: s.features(ctx, ch.ID).ViewForumAsMessages[user],
	}).To_Dialog()
	return mtproto.MakeTLUpdatesChannelDifferenceTooLong(&mtproto.Updates_ChannelDifference{
		Final:    true,
		Timeout:  timeout,
		Dialog:   dlg,
		Messages: msgs,
		Chats:    []*mtproto.Chat{s.ChatFor(ctx, user, ch)},
		Users:    []*mtproto.User{},
	}).To_Updates_ChannelDifference()
}

func mustMessages(s *Service, ctx context.Context, channelID int64) []*StoredMessage {
	all, _ := s.store.ListMessages(ctx, channelID)
	return all
}

// LegacyMember is a basic-group participant being migrated.
type LegacyMember struct {
	UserID int64
	IsBot  bool
}

// MigrateRequest converts a legacy chat into a supergroup.
type MigrateRequest struct {
	Actor     int64
	ActorAuth int64
	ChatID    int64
	Title     string
	About     string
	PhotoID   int64
	CreatorID int64
	Members   []LegacyMember
}

// MigrateResult is the new supergroup plus the updates returned to the actor.
type MigrateResult struct {
	Updates    *mtproto.Updates
	ChannelID  int64
	AccessHash int64
	Already    bool
}

func (s *Service) Migrate(ctx context.Context, in MigrateRequest) (*MigrateResult, error) {
	if in.Actor == 0 || in.Actor != in.CreatorID {
		return nil, mtproto.ErrChatAdminRequired
	}
	if existing, err := s.store.FindByMigratedChat(ctx, in.ChatID); err != nil {
		return nil, err
	} else if existing != nil {
		up := s.pack(ctx, in.Actor, existing.ID, channelUpdate(existing.ID))
		return &MigrateResult{Updates: up, ChannelID: existing.ID, AccessHash: existing.AccessHash, Already: true}, nil
	}
	now := int32(time.Now().Unix())
	ch := &Channel{
		AccessHash:         randInt63(),
		CreatorID:          in.CreatorID,
		Title:              in.Title,
		About:              in.About,
		PhotoID:            in.PhotoID,
		Megagroup:          true,
		Date:               now,
		MigratedFromChatID: in.ChatID,
		DefaultBanned:      defaultChannelBanned(),
		ParticipantsCount:  1,
		Version:            1,
	}
	if err := s.store.CreateChannel(ctx, ch, &Participant{
		UserID:      in.CreatorID,
		Role:        RoleCreator,
		State:       StateNormal,
		AdminRights: allAdminRights(),
		Date:        now,
	}); err != nil {
		return nil, err
	}
	for _, m := range in.Members {
		if m.UserID == 0 || m.UserID == in.CreatorID {
			continue
		}
		_ = s.store.PutParticipant(ctx, &Participant{
			ChannelID: ch.ID,
			UserID:    m.UserID,
			Role:      RoleMember,
			State:     StateNormal,
			InviterID: in.CreatorID,
			IsBot:     m.IsBot,
			Date:      now,
		})
	}
	if _, err := s.refreshCount(ctx, ch.ID); err != nil {
		return nil, err
	}
	msg := serviceMessage(in.Actor, ch.ID, mtproto.MakeMessageActionChannelMigrateFrom(in.Title, in.ChatID))
	if _, _, err := s.appendMessage(ctx, ch.ID, in.Actor, 0, msg); err != nil {
		return nil, err
	}
	// The migrated history ends at this service message.
	reloaded, err := s.reload(ctx, ch.ID)
	if err != nil {
		return nil, err
	}
	reloaded.MigratedFromMaxID = reloaded.TopMessage
	if err := s.store.SaveChannel(ctx, reloaded); err != nil {
		return nil, err
	}
	updates, err := s.actorUpdates(ctx, in.Actor, in.ActorAuth, ch.ID, KindNew)
	if err != nil {
		return nil, err
	}
	return &MigrateResult{
		Updates:    updates,
		ChannelID:  ch.ID,
		AccessHash: ch.AccessHash,
		Already:    false,
	}, nil
}

// DialogRow is one channel dialog for messages.getDialogs.
type DialogRow struct {
	ChannelID           int64
	TopMessage          int32
	ReadInbox           int32
	ReadOutbox          int32
	Unread              int32
	Date                int32
	Pts                 int32
	ViewForumAsMessages bool
}

func (s *Service) DialogsFor(ctx context.Context, userID int64) ([]DialogRow, error) {
	list, err := s.store.ListByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]DialogRow, 0, len(list))
	for _, ch := range list {
		st, _ := s.store.GetRead(ctx, ch.ID, userID)
		if st == nil {
			st = &ReadState{}
		}
		date := ch.Date
		if msg, _ := s.store.GetMessage(ctx, ch.ID, ch.TopMessage); msg != nil {
			date = msg.Date
		}
		out = append(out, DialogRow{
			ChannelID:           ch.ID,
			TopMessage:          ch.TopMessage,
			ReadInbox:           st.ReadInboxMaxID,
			ReadOutbox:          st.ReadOutboxMaxID,
			Unread:              st.UnreadCount,
			Date:                date,
			Pts:                 ch.Pts,
			ViewForumAsMessages: s.features(ctx, ch.ID).ViewForumAsMessages[userID],
		})
	}
	return out, nil
}

func (s *Service) MessageBox(ctx context.Context, viewer, channelID int64, id int32) (*mtproto.MessageBox, error) {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return nil, err
	}
	if _, _, err = s.requireView(ctx, viewer, &mtproto.InputChannel{
		PredicateName: mtproto.Predicate_inputChannel,
		ChannelId:     ch.ID,
		AccessHash:    ch.AccessHash,
	}); err != nil {
		return nil, err
	}
	stored, err := s.store.GetMessage(ctx, channelID, id)
	if err != nil || stored == nil || stored.Deleted {
		return nil, mtproto.ErrMessageIdInvalid
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return nil, err
	}
	return mtproto.MakeTLMessageBox(&mtproto.MessageBox{
		UserId:       viewer,
		MessageId:    id,
		SenderUserId: stored.SenderID,
		PeerType:     mtproto.PEER_CHANNEL,
		PeerId:       channelID,
		Message:      body,
	}).To_MessageBox(), nil
}

func (s *Service) ResolveUsername(ctx context.Context, viewer int64, username string) (*mtproto.Chat, error) {
	ch, err := s.store.FindByUsername(ctx, username)
	if err != nil {
		return nil, err
	}
	if ch == nil || ch.Deleted {
		return nil, mtproto.ErrUsernameNotOccupied
	}
	return s.ChatFor(ctx, viewer, ch), nil
}

func (s *Service) ChatsFor(ctx context.Context, viewer int64, ids ...int64) []*mtproto.Chat {
	out := make([]*mtproto.Chat, 0, len(ids))
	for _, id := range ids {
		ch, err := s.store.GetChannel(ctx, id)
		if err != nil || ch == nil {
			continue
		}
		out = append(out, s.ChatFor(ctx, viewer, ch))
	}
	return out
}

func (s *Service) actorUpdates(ctx context.Context, actor, actorAuth, channelID int64, kind string) (*mtproto.Updates, error) {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return nil, err
	}
	stored, err := s.store.GetMessage(ctx, channelID, ch.TopMessage)
	if err != nil || stored == nil {
		return s.pack(ctx, actor, channelID, channelUpdate(channelID)), nil
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return nil, err
	}
	var upd *mtproto.Update
	view := viewMessage(body, stored.SenderID, actor)
	switch kind {
	case KindEdit:
		upd = editMessageUpdate(view, ch.Pts, 1)
	default:
		upd = newMessageUpdate(view, stored.Pts, 1)
	}
	updates := s.updatesWith(ctx, actor, ch, upd, channelUpdate(channelID))
	s.emit(ctx, actor, actorAuth, channelID, func(viewer int64) *mtproto.Updates {
		v := viewMessage(body, stored.SenderID, viewer)
		var u *mtproto.Update
		if kind == KindEdit {
			u = editMessageUpdate(v, ch.Pts, 1)
		} else {
			u = newMessageUpdate(v, stored.Pts, 1)
		}
		return s.updatesWith(ctx, viewer, ch, u, channelUpdate(channelID))
	})
	return updates, nil
}

func (s *Service) pack(ctx context.Context, viewer, channelID int64, ups ...*mtproto.Update) *mtproto.Updates {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return mtproto.MakeUpdatesByUpdates(ups...)
	}
	return s.updatesWith(ctx, viewer, ch, ups...)
}

func (s *Service) updatesWith(ctx context.Context, viewer int64, ch *Channel, ups ...*mtproto.Update) *mtproto.Updates {
	if len(ups) == 0 {
		ups = []*mtproto.Update{}
	}
	return mtproto.MakeUpdatesByUpdatesChats([]*mtproto.Chat{s.ChatFor(ctx, viewer, ch)}, ups...)
}

func (s *Service) emit(ctx context.Context, actor, actorAuth, channelID int64, build func(viewer int64) *mtproto.Updates) {
	h := s.hooks()
	if h.Push == nil && h.Dialog == nil {
		return
	}
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return
	}
	parts, err := s.store.ListParticipants(ctx, channelID)
	if err != nil {
		return
	}
	date := int64(ch.Date)
	if msg, _ := s.store.GetMessage(ctx, channelID, ch.TopMessage); msg != nil {
		date = int64(msg.Date)
	}
	for _, p := range parts {
		if p.State != StateNormal {
			continue
		}
		if h.Push != nil && build != nil {
			var excl []int64
			if p.UserID == actor && actorAuth != 0 {
				excl = []int64{actorAuth}
			}
			h.Push(ctx, p.UserID, excl, build(p.UserID))
		}
		if h.Dialog != nil {
			st, _ := s.store.GetRead(ctx, channelID, p.UserID)
			if st == nil {
				st = &ReadState{}
			}
			h.Dialog(ctx, p.UserID, channelID, ch.TopMessage, st.ReadInboxMaxID, st.ReadOutboxMaxID, st.UnreadCount, date)
		}
	}
}
