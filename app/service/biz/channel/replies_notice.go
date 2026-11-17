package channel

import (
	"context"
	"encoding/binary"
	"hash/fnv"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// RepliesUserID is the @replies service user. Official clients read discussion
// reply notifications from the private chat with this account, and
// contacts.blockFromReplies addresses that chat by message id.
// https://core.telegram.org/api/discussion
const RepliesUserID int64 = 1271266957

// RepliesNotice is one inbox message for the author of a discussion comment
// that was just replied to. Message.Id is 0: the user inbox assigns its own
// box id. CommentID is the id in the discussion group, not the inbox id.
// RandomID is stable for (recipient, group, comment) and is the idempotency
// key. A retry must not insert a second row.
type RepliesNotice struct {
	Recipient int64
	Commenter int64
	GroupID   int64
	CommentID int32
	ParentID  int32
	ThreadID  int32
	RandomID  int64
	Message   *mtproto.Message
}

// RepliesDelivery is what the inbox writer does with one notice.
// Write false means the recipient must not receive the text or the author.
// Push false still allows the row to be stored for the Replies chat, and
// does not send a live notification that carries the text or the author.
type RepliesDelivery struct {
	Write  bool
	Push   bool
	Silent bool
}

// RepliesDeliveryOf applies block and notification privacy.
// A blocked commenter produces no row and no push.
// show_previews set to false stores a silent row and does not push, so a
// notification does not carry the comment text or the commenter.
// silent or a mute that is still in the future marks the row silent and
// still pushes it to that user only: the Replies chat updates, and the
// message itself asks clients not to raise a banner.
// now is unix time. A nil settings list is the default (deliver, not silent).
func RepliesDeliveryOf(blocked bool, now int32, settings ...*mtproto.PeerNotifySettings) RepliesDelivery {
	if blocked {
		return RepliesDelivery{}
	}
	out := RepliesDelivery{Write: true, Push: true}
	for _, st := range settings {
		if st == nil {
			continue
		}
		if st.ShowPreviews != nil && !mtproto.FromBool(st.ShowPreviews) {
			out.Silent = true
			out.Push = false
		}
		if mtproto.FromBool(st.Silent) {
			out.Silent = true
		}
		if st.MuteUntil != nil && st.MuteUntil.GetValue() > now {
			out.Silent = true
		}
	}
	return out
}

// writeRepliesNotice runs after a channel message is stored. It does not
// turn TELLOMI_CHANNEL_COMMENTS on. A missing hook, or a message that is
// not a reply to another user in a linked discussion group, is success.
// A store or hook error is retried three times. The comment row is already
// committed; RandomID makes the inbox write idempotent.
func (s *Service) writeRepliesNotice(ctx context.Context, groupID int64, commentID int32) error {
	if s == nil || s.hooks().RepliesInbox == nil {
		return nil
	}
	var last error
	for attempt := 0; attempt < 3; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		n, ok, err := s.prepareRepliesNotice(ctx, groupID, commentID)
		if err != nil {
			last = err
			continue
		}
		if !ok {
			return nil
		}
		h := s.hooks().RepliesInbox
		if h == nil {
			return nil
		}
		last = h(ctx, n)
		if last == nil {
			return nil
		}
	}
	return last
}

func (s *Service) prepareRepliesNotice(ctx context.Context, groupID int64, commentID int32) (RepliesNotice, bool, error) {
	var zero RepliesNotice
	if s == nil || groupID == 0 || commentID == 0 {
		return zero, false, nil
	}
	group, err := s.reload(ctx, groupID)
	if err != nil {
		if err == mtproto.ErrChannelInvalid {
			return zero, false, nil
		}
		return zero, false, err
	}
	if group == nil || !group.Megagroup || group.Restricted || group.Banned {
		return zero, false, nil
	}
	linkID := s.features(ctx, groupID).LinkedChatID
	if linkID == 0 {
		return zero, false, nil
	}
	linked, err := s.reload(ctx, linkID)
	if err != nil {
		if err == mtproto.ErrChannelInvalid {
			return zero, false, nil
		}
		return zero, false, err
	}
	if linked == nil || !linked.Broadcast || linked.Deleted || linked.Restricted || linked.Banned {
		return zero, false, nil
	}
	stored, err := s.store.GetMessage(ctx, groupID, commentID)
	if err != nil {
		return zero, false, err
	}
	if stored == nil || stored.Deleted {
		return zero, false, nil
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return zero, false, err
	}
	if isServiceMessage(body) || len(body.GetRestrictionReason()) > 0 {
		return zero, false, nil
	}
	if !s.inCommentSection(ctx, groupID, stored, body) {
		return zero, false, nil
	}
	parentID := replyMsgID(body)
	if parentID == 0 || parentID == commentID {
		return zero, false, nil
	}
	parent, err := s.store.GetMessage(ctx, groupID, parentID)
	if err != nil {
		return zero, false, err
	}
	if parent == nil || parent.Deleted {
		return zero, false, nil
	}
	parentBody, err := unmarshalMessage(parent.Data)
	if err != nil {
		return zero, false, err
	}
	recipient := noticeAuthor(parent, parentBody)
	commenter := noticeAuthor(stored, body)
	if recipient == 0 || commenter == 0 || recipient == commenter || recipient == RepliesUserID || commenter == RepliesUserID {
		return zero, false, nil
	}
	ok, err := s.repliesReadable(ctx, group, recipient, parent, stored)
	if err != nil || !ok {
		return zero, false, err
	}
	threadID := replyTopID(body)
	msg := proto.Clone(body).(*mtproto.Message)
	msg.Id = 0
	msg.Out = false
	msg.Post = false
	msg.Mentioned = true
	msg.FromId = mtproto.MakePeerUser(RepliesUserID)
	msg.PeerId = mtproto.MakePeerUser(RepliesUserID)
	msg.Views = nil
	msg.Forwards = nil
	msg.Replies = nil
	msg.EditDate = nil
	msg.Reactions = nil
	msg.ReplyMarkup = nil
	msg.RestrictionReason = nil
	msg.PostAuthor = nil
	msg.SavedPeerId = nil
	if msg.PredicateName == "" {
		msg.PredicateName = mtproto.Predicate_message
	}
	if s.features(ctx, groupID).NoForwards {
		msg.Noforwards = true
	}
	date := stored.Date
	if date == 0 {
		date = body.GetDate()
	}
	msg.Date = date
	msg.FwdFrom = mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
		FromId:         mtproto.MakePeerUser(commenter),
		Date:           date,
		SavedFromPeer:  mtproto.MakePeerChannel(groupID),
		SavedFromMsgId: wrapperspb.Int32(commentID),
	}).To_MessageFwdHeader()
	header := &mtproto.MessageReplyHeader{
		ReplyToMsgId:           parentID,
		ReplyToMsgId_INT32:     parentID,
		ReplyToMsgId_FLAGINT32: wrapperspb.Int32(parentID),
		ReplyToPeerId:          mtproto.MakePeerChannel(groupID),
	}
	if threadID != 0 {
		header.ReplyToTopId = wrapperspb.Int32(threadID)
	}
	msg.ReplyTo = mtproto.MakeTLMessageReplyHeader(header).To_MessageReplyHeader()
	return RepliesNotice{
		Recipient: recipient,
		Commenter: commenter,
		GroupID:   groupID,
		CommentID: commentID,
		ParentID:  parentID,
		ThreadID:  threadID,
		RandomID:  repliesRandomID(recipient, groupID, commentID),
		Message:   msg,
	}, true, nil
}

// inCommentSection is true for a reply that belongs to a channel post's
// comment thread. While no thread index exists (TELLOMI_CHANNEL_COMMENTS
// is off, so channel posts are not forwarded), a reply in the linked
// discussion group is the comment this server can see. Once the index
// exists, a reply in the group's general chat is not a comment.
func (s *Service) inCommentSection(ctx context.Context, groupID int64, stored *StoredMessage, body *mtproto.Message) bool {
	f := s.features(ctx, groupID)
	if len(f.CommentByRoot) == 0 && len(f.CommentMsgRoot) == 0 {
		return true
	}
	if stored != nil && f.CommentMsgRoot[stored.ID] != 0 {
		return true
	}
	parent := replyMsgID(body)
	if parent != 0 {
		if _, ok := f.CommentByRoot[parent]; ok {
			return true
		}
		if f.CommentMsgRoot[parent] != 0 {
			return true
		}
	}
	if top := replyTopID(body); top != 0 {
		if _, ok := f.CommentByRoot[top]; ok {
			return true
		}
	}
	return false
}

// repliesReadable is the read check for the parent author. A kicked member,
// a view ban, hidden history from before they joined, and a non-member who
// cannot view the linked channel get nothing. A non-member who can still
// view the channel is the case @replies exists for.
func (s *Service) repliesReadable(ctx context.Context, group *Channel, recipient int64, parent, comment *StoredMessage) (bool, error) {
	p, err := s.store.GetParticipant(ctx, group.ID, recipient)
	if err != nil {
		return false, err
	}
	if p != nil && p.State == StateKicked {
		return false, nil
	}
	if blocked, _ := s.viewBlocked(p); blocked {
		return false, nil
	}
	f := s.features(ctx, group.ID)
	if prehistoryBlocks(f, p, parent) || prehistoryBlocks(f, p, comment) {
		return false, nil
	}
	if p != nil && p.InChannel() {
		return true, nil
	}
	link := f.LinkedChatID
	if link == 0 {
		return false, nil
	}
	if err = s.canViewChannel(ctx, link, recipient); err != nil {
		if err == mtproto.ErrChannelPrivate || err == mtproto.ErrUserBannedInChannel {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// noticeAuthor is the user the message is attributed to. A channel or chat
// from_id is not a user, including an anonymous admin and an auto-forwarded
// channel post. Falling through to the stored sender in those cases would
// name the hidden admin.
func noticeAuthor(stored *StoredMessage, body *mtproto.Message) int64 {
	if body != nil && body.GetFromId() != nil {
		p := body.GetFromId()
		if p.GetPredicateName() == mtproto.Predicate_peerUser && p.GetUserId() != 0 {
			return p.GetUserId()
		}
		return 0
	}
	if stored != nil && stored.SenderID > 0 {
		return stored.SenderID
	}
	return 0
}

func repliesRandomID(recipient, groupID int64, commentID int32) int64 {
	h := fnv.New64a()
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(recipient))
	_, _ = h.Write(buf[:])
	binary.LittleEndian.PutUint64(buf[:], uint64(groupID))
	_, _ = h.Write(buf[:])
	binary.LittleEndian.PutUint32(buf[:4], uint32(commentID))
	_, _ = h.Write(buf[:4])
	v := int64(h.Sum64() & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}
