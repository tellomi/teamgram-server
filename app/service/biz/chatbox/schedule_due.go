package chatbox

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// ScheduleWhenOnline is messages.sendMessage schedule_date 0x7FFFFFFE:
// deliver when the peer comes online.
// https://core.telegram.org/api/scheduled-messages
const ScheduleWhenOnline int32 = 0x7FFFFFFE

// SchedIDBase keeps new scheduled ids out of the live message box.
// A live box id and a scheduled id used to be the same small integer, so
// messages.editMessage treated the scheduled id as someone else's message
// and returned MESSAGE_AUTHOR_REQUIRED.
const SchedIDBase int32 = 1_000_000_000

// Repeat periods official clients send. Anything else is SCHEDULE_DATE_INVALID.
// https://core.telegram.org/api/scheduled-messages
// 60 and 300 exist only on the Test DC and are not accepted here.
var repeatPeriods = map[int32]struct{}{
	86400:    {}, // 1 day
	604800:   {}, // 1 week
	1209600:  {}, // 2 weeks
	2592000:  {}, // 30 days
	7862400:  {}, // 91 days
	15724800: {}, // 182 days
	31536000: {}, // 365 days
}

// RepeatSingleOnly rejects schedule_repeat_period on an album or a forward
// of more than one message. The official method page allows the period only
// when a single message is sent or forwarded.
func RepeatSingleOnly(count int, period int32) error {
	if period != 0 && count != 1 {
		return mtproto.ErrScheduleDateInvalid
	}
	return nil
}

// SendInsteadOfSchedule is the official short window: a schedule_date less
// than 10 seconds ahead is sent immediately, as a normal message.
// A repeating message stays in the queue so the following occurrence is kept.
func SendInsteadOfSchedule(when, now int32, repeating bool) bool {
	if repeating || when == 0 || when == ScheduleWhenOnline || when <= now {
		return false
	}
	return when-now < 10
}

func ValidRepeatPeriod(period int32) bool {
	_, ok := repeatPeriods[period]
	return ok
}

// DispatchEnabled is TELLOMI_SCHEDULED_DISPATCH. The due sender is off
// unless this is 1 or true, so a process does not deliver stored messages
// until the operator turns it on.
func DispatchEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TELLOMI_SCHEDULED_DISPATCH"))) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func checkRepeat(msg *mtproto.Message, online bool) error {
	period := msg.GetScheduleRepeatPeriod().GetValue()
	if period == 0 {
		return nil
	}
	if online || !ValidRepeatPeriod(period) {
		return mtproto.ErrScheduleDateInvalid
	}
	return nil
}

type dueRef struct {
	Owner    int64 `json:"o"`
	PeerType int32 `json:"t"`
	PeerID   int64 `json:"p"`
	ID       int32 `json:"id"`
	When     int32 `json:"w"`
	Online   bool  `json:"on,omitempty"`
	Retry    int32 `json:"r,omitempty"`
	Tries    int32 `json:"n,omitempty"`
}

func dueIndexKey() string { return "sched:due" }

func (b *Book) addDueLocked(ctx context.Context, refs []dueRef) error {
	if len(refs) == 0 {
		return nil
	}
	rows, err := b.loadDueLocked(ctx)
	if err != nil {
		return err
	}
	rows = append(rows, refs...)
	return b.saveDueLocked(ctx, rows)
}

func (b *Book) dropDueLocked(ctx context.Context, owner int64, peer *mtproto.PeerUtil, ids []int32) error {
	if peer == nil || len(ids) == 0 {
		return nil
	}
	rows, err := b.loadDueLocked(ctx)
	if err != nil {
		return err
	}
	drop := map[int32]bool{}
	for _, id := range ids {
		drop[id] = true
	}
	kept := rows[:0]
	for _, row := range rows {
		if row.Owner == owner && row.PeerType == peer.PeerType && row.PeerID == peer.PeerId && drop[row.ID] {
			continue
		}
		kept = append(kept, row)
	}
	return b.saveDueLocked(ctx, kept)
}

func (b *Book) retimedDueLocked(ctx context.Context, owner int64, peer *mtproto.PeerUtil, id, when int32, online bool) error {
	rows, err := b.loadDueLocked(ctx)
	if err != nil {
		return err
	}
	found := false
	for i := range rows {
		if rows[i].Owner == owner && rows[i].PeerType == peer.PeerType && rows[i].PeerID == peer.PeerId && rows[i].ID == id {
			rows[i].When = when
			rows[i].Online = online
			found = true
			break
		}
	}
	if !found {
		rows = append(rows, dueRef{Owner: owner, PeerType: peer.PeerType, PeerID: peer.PeerId, ID: id, When: when, Online: online})
	}
	return b.saveDueLocked(ctx, rows)
}

func (b *Book) loadDueLocked(ctx context.Context) ([]dueRef, error) {
	var rows []dueRef
	if err := b.load(ctx, dueIndexKey(), &rows); err != nil {
		return nil, err
	}
	if b.db == nil {
		if len(b.due) > 0 {
			return append([]dueRef(nil), b.due...), nil
		}
		return []dueRef{}, nil
	}
	if rows == nil {
		rows = []dueRef{}
	}
	b.due = rows
	return rows, nil
}

func (b *Book) saveDueLocked(ctx context.Context, rows []dueRef) error {
	if rows == nil {
		rows = []dueRef{}
	}
	b.due = rows
	return b.save(ctx, dueIndexKey(), rows)
}

// HasScheduled reports whether this user has that scheduled id in the dialog.
func (b *Book) HasScheduled(ctx context.Context, owner int64, peer *mtproto.PeerUtil, id int32) (bool, error) {
	msg, err := b.scheduledOne(ctx, owner, peer, id)
	return msg != nil, err
}

func (b *Book) scheduledOne(ctx context.Context, owner int64, peer *mtproto.PeerUtil, id int32) (*mtproto.Message, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() || id == 0 {
		return nil, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.loadSched(ctx, schedKey(owner, peer.PeerType, peer.PeerId))
	if err != nil {
		return nil, err
	}
	for _, item := range doc.Items {
		if item.ID != id {
			continue
		}
		msg := &mtproto.Message{}
		if err := proto.Unmarshal(item.Msg, msg); err != nil {
			return nil, err
		}
		return msg, nil
	}
	return nil, nil
}

// SchedEdit is one messages.editMessage applied to a scheduled message.
// Nil fields keep the stored value. When 0 is not a new date; the caller
// passes ScheduleWhenOnline or a future unix time when the date changes.
type SchedEdit struct {
	Text     *string
	Media    *mtproto.MessageMedia
	Entities []*mtproto.MessageEntity
	Markup   *mtproto.ReplyMarkup
	When     *int32
	Repeat   *int32
	Now      int32
}

// EditScheduled rewrites one scheduled message and returns
// updateNewScheduledMessage. A missing id is MESSAGE_ID_INVALID.
func (b *Book) EditScheduled(ctx context.Context, owner int64, peer *mtproto.PeerUtil, id int32, edit SchedEdit) (*mtproto.Updates, error) {
	if peer == nil || !peer.IsUserOrChatOrChannel() {
		return nil, mtproto.ErrPeerIdInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	key := schedKey(owner, peer.PeerType, peer.PeerId)
	doc, err := b.loadSched(ctx, key)
	if err != nil {
		return nil, err
	}
	idx := -1
	var msg *mtproto.Message
	for i, item := range doc.Items {
		if item.ID != id {
			continue
		}
		msg = &mtproto.Message{}
		if err := proto.Unmarshal(item.Msg, msg); err != nil {
			return nil, err
		}
		idx = i
		break
	}
	if idx < 0 || msg == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	when := msg.GetDate()
	if edit.When != nil {
		when = *edit.When
	}
	online := when == ScheduleWhenOnline
	if online {
		if peer.PeerType != mtproto.PEER_USER || peer.PeerId == 0 || peer.PeerId == owner {
			return nil, mtproto.ErrScheduleStatusPrivate
		}
	} else {
		now := edit.Now
		if now == 0 {
			now = int32(b.now().Unix())
		}
		if when <= now {
			return nil, mtproto.ErrScheduleDateInvalid
		}
		if time.Unix(int64(when), 0).After(time.Unix(int64(now), 0).Add(maxScheduleAhead)) {
			return nil, mtproto.ErrScheduleDateTooLate
		}
	}
	if edit.Text != nil {
		msg.Message = *edit.Text
	}
	if edit.Media != nil {
		msg.Media = edit.Media
	}
	if edit.Entities != nil {
		msg.Entities = edit.Entities
	}
	if edit.Markup != nil {
		msg.ReplyMarkup = edit.Markup
	}
	if edit.Repeat != nil {
		msg.ScheduleRepeatPeriod = nil
		if *edit.Repeat != 0 {
			msg.ScheduleRepeatPeriod = &wrapperspb.Int32Value{Value: *edit.Repeat}
		}
	}
	if err := checkRepeat(msg, online); err != nil {
		return nil, err
	}
	if msg.Message == "" && msg.Media == nil {
		return nil, mtproto.ErrMessageEmpty
	}
	msg.Date = when
	msg.EditDate = &wrapperspb.Int32Value{Value: edit.Now}
	if edit.Now == 0 {
		msg.EditDate = &wrapperspb.Int32Value{Value: int32(b.now().Unix())}
	}
	raw, err := proto.Marshal(msg)
	if err != nil {
		return nil, err
	}
	doc.Items[idx].When = when
	doc.Items[idx].Msg = raw
	b.sched[key] = doc
	if err := b.save(ctx, key, doc); err != nil {
		return nil, err
	}
	if err := b.retimedDueLocked(ctx, owner, peer, id, when, online); err != nil {
		return nil, err
	}
	up := mtproto.MakeTLUpdateNewScheduledMessage(&mtproto.Update{Message_MESSAGE: msg}).To_Update()
	return mtproto.MakeUpdatesByUpdates(up), nil
}

// DueItem is one scheduled message whose send time has arrived, already
// removed from the inbox. Repeat is the period to queue again after a
// successful send. Restore it with RestoreScheduled if the send fails.
type DueItem struct {
	Owner   int64
	Peer    *mtproto.PeerUtil
	SchedID int32
	Message *mtproto.Message
	Repeat  int32
	When    int32
	Tries   int32
}

// ClaimDue removes scheduled messages that should be sent now.
// online is the set of user ids that are online; send-when-online messages
// for those peers are included. A restarted process reads the same rows
// from the doc store.
func (b *Book) ClaimDue(ctx context.Context, now int32, online map[int64]bool, limit int) ([]DueItem, error) {
	if limit <= 0 {
		limit = 50
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	rows, err := b.loadDueLocked(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 && !b.dueIndexed {
		rows, err = b.rebuildDueLocked(ctx)
		if err != nil {
			return nil, err
		}
		b.dueIndexed = true
	}
	var picked []dueRef
	kept := make([]dueRef, 0, len(rows))
	for _, row := range rows {
		ready := false
		if row.Retry > now {
			ready = false
		} else if row.Online || row.When == ScheduleWhenOnline {
			ready = online[row.PeerID]
		} else if row.When > 0 && row.When <= now {
			ready = true
		}
		if ready && len(picked) < limit {
			picked = append(picked, row)
			continue
		}
		kept = append(kept, row)
	}
	if len(picked) == 0 {
		return nil, nil
	}
	var out []DueItem
	for _, row := range picked {
		peer := mtproto.MakePeerUtil(row.PeerType, row.PeerID)
		key := schedKey(row.Owner, row.PeerType, row.PeerID)
		doc, err := b.loadSched(ctx, key)
		if err != nil {
			b.restoreClaimedLocked(ctx, out)
			return nil, err
		}
		var msg *mtproto.Message
		rest := doc.Items[:0]
		prev := append([]schedItem(nil), doc.Items...)
		for _, item := range doc.Items {
			if item.ID != row.ID || msg != nil {
				rest = append(rest, item)
				continue
			}
			msg = &mtproto.Message{}
			if err := proto.Unmarshal(item.Msg, msg); err != nil {
				b.restoreClaimedLocked(ctx, out)
				return nil, err
			}
		}
		doc.Items = rest
		b.sched[key] = doc
		if err := b.save(ctx, key, doc); err != nil {
			doc.Items = prev
			b.sched[key] = doc
			b.restoreClaimedLocked(ctx, out)
			return nil, err
		}
		if msg == nil {
			continue
		}
		out = append(out, DueItem{
			Owner:   row.Owner,
			Peer:    peer,
			SchedID: row.ID,
			Message: msg,
			Repeat:  msg.GetScheduleRepeatPeriod().GetValue(),
			When:    row.When,
			Tries:   row.Tries,
		})
	}
	if err := b.saveDueLocked(ctx, kept); err != nil {
		b.restoreClaimedLocked(ctx, out)
		return nil, err
	}
	return out, nil
}

// restoreClaimedLocked puts claimed messages back. The book lock is held.
func (b *Book) restoreClaimedLocked(ctx context.Context, items []DueItem) {
	for _, item := range items {
		if item.Message == nil || item.Peer == nil {
			continue
		}
		key := schedKey(item.Owner, item.Peer.PeerType, item.Peer.PeerId)
		doc, err := b.loadSched(ctx, key)
		if err != nil {
			continue
		}
		raw, err := proto.Marshal(item.Message)
		if err != nil {
			continue
		}
		doc.Items = append(doc.Items, schedItem{ID: item.Message.Id, When: item.Message.Date, Msg: raw})
		if item.Message.Id > doc.Next {
			doc.Next = item.Message.Id
		}
		b.sched[key] = doc
		_ = b.save(ctx, key, doc)
	}
}

// ScheduleNext queues the following occurrence of a repeating message.
// The new date is the send time plus the period, which is how the API
// describes the re-schedule ("current time plus schedule_repeat_period").
func (b *Book) ScheduleNext(ctx context.Context, item DueItem, now int32) (*mtproto.Message, error) {
	if item.Message == nil || item.Peer == nil || !ValidRepeatPeriod(item.Repeat) {
		return nil, nil
	}
	wall := int32(b.now().Unix())
	if now > wall {
		wall = now
	}
	nextWhen := wall + item.Repeat
	cp := proto.Clone(item.Message).(*mtproto.Message)
	cp.Id = 0
	cp.FromScheduled = false
	cp.EditDate = nil
	cp.ScheduleRepeatPeriod = &wrapperspb.Int32Value{Value: item.Repeat}
	ups, err := b.Schedule(ctx, item.Owner, item.Peer, nextWhen, []*mtproto.Message{cp})
	if err != nil || ups == nil || len(ups.Updates) == 0 {
		return nil, err
	}
	return ups.Updates[0].GetMessage_MESSAGE(), nil
}

func (b *Book) rebuildDueLocked(ctx context.Context) ([]dueRef, error) {
	lister, ok := b.db.(interface {
		ListPrefix(context.Context, string) ([]string, error)
	})
	if !ok {
		return nil, nil
	}
	keys, err := lister.ListPrefix(ctx, "sched:")
	if err != nil {
		return nil, err
	}
	var rows []dueRef
	for _, key := range keys {
		if key == dueIndexKey() || !strings.HasPrefix(key, "sched:") {
			continue
		}
		owner, peerType, peerID, ok := parseSchedKey(key)
		if !ok {
			continue
		}
		doc, err := b.loadSched(ctx, key)
		if err != nil {
			return nil, err
		}
		for _, item := range doc.Items {
			rows = append(rows, dueRef{
				Owner:    owner,
				PeerType: peerType,
				PeerID:   peerID,
				ID:       item.ID,
				When:     item.When,
				Online:   item.When == ScheduleWhenOnline,
			})
		}
	}
	if err := b.saveDueLocked(ctx, rows); err != nil {
		return nil, err
	}
	return rows, nil
}

func parseSchedKey(key string) (owner int64, peerType int32, peerID int64, ok bool) {
	// sched:{owner}:{peerType}:{peerID}
	rest, found := strings.CutPrefix(key, "sched:")
	if !found {
		return 0, 0, 0, false
	}
	parts := strings.Split(rest, ":")
	if len(parts) != 3 {
		return 0, 0, 0, false
	}
	o, err1 := parseI64(parts[0])
	t, err2 := parseI64(parts[1])
	p, err3 := parseI64(parts[2])
	if err1 != nil || err2 != nil || err3 != nil {
		return 0, 0, 0, false
	}
	return o, int32(t), p, true
}

func parseI64(s string) (int64, error) {
	var n int64
	neg := false
	if strings.HasPrefix(s, "-") {
		neg = true
		s = s[1:]
	}
	if s == "" {
		return 0, mtproto.ErrInternalServerError
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, mtproto.ErrInternalServerError
		}
		n = n*10 + int64(c-'0')
	}
	if neg {
		n = -n
	}
	return n, nil
}

// OnlinePeers lists user ids that have a send-when-online message waiting.
func (b *Book) OnlinePeers(ctx context.Context) ([]int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	rows, err := b.loadDueLocked(ctx)
	if err != nil {
		return nil, err
	}
	seen := map[int64]bool{}
	var ids []int64
	for _, row := range rows {
		if !row.Online && row.When != ScheduleWhenOnline {
			continue
		}
		if row.PeerID == 0 || seen[row.PeerID] {
			continue
		}
		seen[row.PeerID] = true
		ids = append(ids, row.PeerID)
	}
	return ids, nil
}

// DueSink delivers one due scheduled message and pushes the resulting
// updates. Send must assign a live message id. Push reaches one user's sessions.
type DueSink interface {
	// randomID is stable for one scheduled message so a retried or duplicated
	// send hits the messenger idempotency key instead of posting twice.
	Send(ctx context.Context, owner int64, peer *mtproto.PeerUtil, msg *mtproto.Message, randomID int64) (sentID int32, err error)
	Push(ctx context.Context, userID int64, ups *mtproto.Updates) error
}

// ScheduleRandom is the idempotency key for one scheduled send.
func ScheduleRandom(owner int64, peer *mtproto.PeerUtil, schedID int32) int64 {
	var peerID int64
	var peerType int32
	if peer != nil {
		peerID = peer.PeerId
		peerType = peer.PeerType
	}
	n := owner*131 + peerID*17 + int64(schedID) + int64(peerType)<<32
	if n < 0 {
		n = -n
	}
	if n == 0 {
		n = 1
	}
	return n
}

// PermanentScheduleErr is a send failure that will not succeed on retry:
// the sender was kicked, blocked, or has no posting rights.
func PermanentScheduleErr(err error) bool {
	if err == nil {
		return false
	}
	msg := status.Convert(err).Message()
	switch msg {
	case "USER_IS_BLOCKED", "YOU_BLOCKED_USER", "PEER_ID_INVALID", "INPUT_USER_DEACTIVATED",
		"USER_BANNED_IN_CHANNEL", "CHAT_WRITE_FORBIDDEN", "CHAT_ADMIN_REQUIRED",
		"USER_NOT_PARTICIPANT", "CHANNEL_PRIVATE", "CHAT_RESTRICTED", "USER_ID_INVALID":
		return true
	}
	return strings.HasPrefix(msg, "CHAT_SEND_") && strings.HasSuffix(msg, "_FORBIDDEN")
}

func scheduleBackoff(tries int32) int32 {
	delay := int32(2)
	for i := int32(1); i < tries; i++ {
		if delay >= 900 {
			return 900
		}
		delay *= 2
	}
	if delay > 900 {
		return 900
	}
	return delay
}

// DeferScheduled puts a message back and waits before the next attempt.
func (b *Book) DeferScheduled(ctx context.Context, item DueItem, now int32) {
	if b == nil || item.Message == nil || item.Peer == nil {
		return
	}
	tries := item.Tries + 1
	until := now + scheduleBackoff(tries)
	b.RestoreScheduled(ctx, item.Owner, item.Peer, []*mtproto.Message{item.Message})
	b.mu.Lock()
	defer b.mu.Unlock()
	rows, err := b.loadDueLocked(ctx)
	if err != nil {
		return
	}
	for i := range rows {
		if rows[i].Owner == item.Owner && rows[i].PeerType == item.Peer.PeerType && rows[i].PeerID == item.Peer.PeerId && rows[i].ID == item.SchedID {
			rows[i].Retry = until
			rows[i].Tries = tries
		}
	}
	_ = b.saveDueLocked(ctx, rows)
}

// DispatchDue sends every due scheduled message.
// The live message is a normal send (from_scheduled set). Both the author
// and the other private-chat participant receive updateDeleteScheduledMessages
// and, when the message repeats, updateNewScheduledMessage for the next time.
// A failed send puts the scheduled message back.
func DispatchDue(ctx context.Context, b *Book, now int32, online map[int64]bool, sink DueSink) (int, error) {
	if b == nil || sink == nil {
		return 0, nil
	}
	items, err := b.ClaimDue(ctx, now, online, 50)
	if err != nil {
		return 0, err
	}
	sent := 0
	for _, item := range items {
		if item.Message == nil || item.Peer == nil {
			continue
		}
		out := proto.Clone(item.Message).(*mtproto.Message)
		out.FromScheduled = true
		out.Date = now
		out.Id = 0
		out.EditDate = nil
		out.ScheduleRepeatPeriod = nil
		id, err := sink.Send(ctx, item.Owner, item.Peer, out, ScheduleRandom(item.Owner, item.Peer, item.SchedID))
		if err != nil || id == 0 {
			if PermanentScheduleErr(err) {
				_ = sink.Push(ctx, item.Owner, mtproto.MakeUpdatesByUpdates(DeleteScheduledUpdate(item.Peer, []int32{item.SchedID}, nil)))
				continue
			}
			b.DeferScheduled(ctx, item, now)
			continue
		}
		var next *mtproto.Message
		if item.Repeat != 0 {
			next, _ = b.ScheduleNext(ctx, item, now)
		}
		// The schedule queue belongs to the sender. The other participant
		// receives the live message from Send. updateDeleteScheduledMessages
		// uses the sender's peer and the sender's new message id, so it must
		// not be copied onto the other user's session.
		// https://core.telegram.org/api/scheduled-messages
		ups := mtproto.MakeUpdatesByUpdates(DeleteScheduledUpdate(item.Peer, []int32{item.SchedID}, []int32{id}))
		if next != nil {
			ups.Updates = append(ups.Updates, mtproto.MakeTLUpdateNewScheduledMessage(&mtproto.Update{
				Message_MESSAGE: next,
			}).To_Update())
		}
		_ = sink.Push(ctx, item.Owner, ups)
		sent++
	}
	return sent, nil
}

// ShouldEditScheduled chooses the scheduled copy over the live box.
// explicit is schedule_date or schedule_repeat_period on the edit.
// A live message the caller wrote, with no schedule field and a low id,
// stays a live edit so the two id spaces can still collide on old rows.
func ShouldEditScheduled(hasSched, explicit bool, id int32, liveFound bool, liveAuthor, self int64) bool {
	if !hasSched {
		return false
	}
	if explicit || id >= SchedIDBase {
		return true
	}
	if !liveFound || liveAuthor != self {
		return true
	}
	return false
}
