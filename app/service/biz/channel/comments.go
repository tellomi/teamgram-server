package channel

import (
	"context"
	"os"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// commentsOn is TELLOMI_CHANNEL_COMMENTS. Default is off: channel posts are
// not forwarded into the linked discussion group, posts have no replies
// field, and a non-member who sends into that group still gets
// CHANNEL_PRIVATE. Set the variable to 1, true, on, or yes to match official
// channel comments.
func commentsOn() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("TELLOMI_CHANNEL_COMMENTS"))) {
	case "1", "true", "on", "yes":
		return true
	default:
		return false
	}
}

func (s *Service) commentBatch(ctx context.Context, ch *Channel, batch []Outgoing) bool {
	if !commentsOn() || ch == nil || len(batch) == 0 {
		return false
	}
	for _, item := range batch {
		if _, _, ok := s.commentTarget(ctx, ch, item.Message); !ok {
			return false
		}
	}
	return true
}

// commentTarget reports the discussion group and the root message a send
// should land on. A megagroup reply to the auto-forward (or to a comment
// under it) stays in that group. A reply to the channel post is moved into
// the linked group.
func (s *Service) commentTarget(ctx context.Context, ch *Channel, msg *mtproto.Message) (groupID int64, rootID int32, ok bool) {
	if !commentsOn() || ch == nil || msg == nil {
		return 0, 0, false
	}
	target := replyMsgID(msg)
	top := replyTopID(msg)
	if ch.Megagroup {
		f := s.features(ctx, ch.ID)
		if top != 0 {
			if _, exists := f.CommentByRoot[top]; exists {
				return ch.ID, top, true
			}
		}
		if target != 0 {
			if _, exists := f.CommentByRoot[target]; exists {
				return ch.ID, target, true
			}
			if root := f.CommentMsgRoot[target]; root != 0 {
				return ch.ID, root, true
			}
		}
		return 0, 0, false
	}
	if !ch.Broadcast {
		return 0, 0, false
	}
	f := s.features(ctx, ch.ID)
	id := target
	if id == 0 {
		id = top
	}
	root, exists := f.CommentRoot[id]
	if !exists || f.LinkedChatID == 0 {
		return 0, 0, false
	}
	return f.LinkedChatID, root, true
}

// tryComment stores one discussion reply. handled is false when this send
// is an ordinary post. Non-members of the discussion group may comment when
// they can view the linked channel; they are not added to the group.
func (s *Service) tryComment(ctx context.Context, ch *Channel, sender, actorAuth int64, item Outgoing) (handled bool, upd *mtproto.Update, groupID int64, err error) {
	groupID, rootID, ok := s.commentTarget(ctx, ch, item.Message)
	if !ok {
		return false, nil, 0, nil
	}
	if err = s.mayComment(ctx, groupID, sender); err != nil {
		return true, nil, groupID, err
	}
	group, err := s.reload(ctx, groupID)
	if err != nil {
		return true, nil, groupID, err
	}
	comment := proto.Clone(item.Message).(*mtproto.Message)
	if comment.PredicateName == "" {
		comment.PredicateName = mtproto.Predicate_message
	}
	comment.ReplyTo = mtproto.MakeTLMessageReplyHeader(&mtproto.MessageReplyHeader{
		ReplyToMsgId:           rootID,
		ReplyToMsgId_INT32:     rootID,
		ReplyToMsgId_FLAGINT32: wrapperspb.Int32(rootID),
		ReplyToTopId:           wrapperspb.Int32(rootID),
	}).To_MessageReplyHeader()
	s.prepareOutgoing(group, sender, comment)
	id, pts, err := s.appendMessage(ctx, groupID, sender, item.RandomID, comment)
	if err != nil {
		return true, nil, groupID, err
	}
	if err = s.noteComment(ctx, groupID, rootID, id, pts, sender); err != nil {
		return true, nil, groupID, err
	}
	stored, err := s.store.GetMessage(ctx, groupID, id)
	if err != nil || stored == nil {
		return true, nil, groupID, mtproto.ErrInternalServerError
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return true, nil, groupID, err
	}
	upd = newMessageUpdate(viewMessage(body, stored.SenderID, sender), pts, 1)
	if groupID != ch.ID {
		s.emit(ctx, sender, actorAuth, groupID, func(viewer int64) *mtproto.Updates {
			return s.updatesWith(ctx, viewer, group, newMessageUpdate(viewMessage(body, stored.SenderID, viewer), pts, 1))
		})
	}
	return true, upd, groupID, nil
}

func (s *Service) mayComment(ctx context.Context, groupID, sender int64) error {
	group, err := s.reload(ctx, groupID)
	if err != nil {
		return err
	}
	if group == nil || !group.Megagroup {
		return mtproto.ErrPeerIdInvalid
	}
	p, err := s.store.GetParticipant(ctx, groupID, sender)
	if err != nil {
		return err
	}
	if p != nil && p.State == StateKicked {
		return mtproto.ErrUserBannedInChannel
	}
	if p != nil && p.InChannel() {
		return s.canPost(group, p)
	}
	link := s.features(ctx, groupID).LinkedChatID
	if link == 0 {
		return mtproto.ErrChannelPrivate
	}
	return s.canViewChannel(ctx, link, sender)
}

func (s *Service) canViewChannel(ctx context.Context, channelID, user int64) error {
	ch, err := s.reload(ctx, channelID)
	if err != nil {
		return err
	}
	p, err := s.store.GetParticipant(ctx, channelID, user)
	if err != nil {
		return err
	}
	if p != nil && p.State == StateKicked {
		return mtproto.ErrUserBannedInChannel
	}
	if p != nil && p.InChannel() {
		if blocked, _ := s.viewBlocked(p); blocked {
			return mtproto.ErrChannelPrivate
		}
		return nil
	}
	if ch.Public() {
		return nil
	}
	return mtproto.ErrChannelPrivate
}

// prefillDiscussionReplies stamps messageReplies onto a broadcast post before
// it is stored, so the new-message pts event already carries comments=true.
func (s *Service) prefillDiscussionReplies(ctx context.Context, ch *Channel, msg *mtproto.Message) {
	group := s.linkedDiscussion(ctx, ch, msg)
	if group == nil {
		return
	}
	msg.Replies = commentReplies(group.ID, group.Pts, 0, nil, 0)
}

// attachDiscussion forwards a new broadcast post into the linked group and
// records the root so later comments can find it.
func (s *Service) attachDiscussion(ctx context.Context, ch *Channel, postID int32) {
	if !commentsOn() || ch == nil || !ch.Broadcast {
		return
	}
	stored, err := s.store.GetMessage(ctx, ch.ID, postID)
	if err != nil || stored == nil || stored.Deleted {
		return
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil || body == nil || isServiceMessage(body) {
		return
	}
	group := s.linkedDiscussion(ctx, ch, body)
	if group == nil {
		stripReplies(ctx, s, ch.ID, postID, body)
		return
	}
	fwd := proto.Clone(body).(*mtproto.Message)
	fwd.Replies = nil
	fwd.Views = nil
	fwd.Forwards = nil
	fwd.EditDate = nil
	fwd.Reactions = nil
	fwd.ReplyTo = nil
	fwd.Post = false
	fwd.FromId = mtproto.MakePeerChannel(ch.ID)
	fwd.PeerId = mtproto.MakePeerChannel(group.ID)
	if fwd.PredicateName == "" || fwd.PredicateName == mtproto.Predicate_messageService {
		fwd.PredicateName = mtproto.Predicate_message
	}
	fwd.FwdFrom = mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
		FromId:         mtproto.MakePeerChannel(ch.ID),
		Date:           body.GetDate(),
		ChannelPost:    wrapperspb.Int32(postID),
		SavedFromPeer:  mtproto.MakePeerChannel(ch.ID),
		SavedFromMsgId: wrapperspb.Int32(postID),
	}).To_MessageFwdHeader()
	rootID, groupPts, err := s.appendMessage(ctx, group.ID, 0, 0, fwd)
	if err != nil {
		stripReplies(ctx, s, ch.ID, postID, body)
		return
	}
	body.Replies = commentReplies(group.ID, groupPts, 0, nil, 0)
	raw, err := proto.Marshal(body)
	if err != nil {
		return
	}
	if err := s.store.ReplaceMessageBody(ctx, ch.ID, postID, raw); err != nil {
		return
	}
	cf := s.features(ctx, ch.ID)
	if cf.CommentRoot == nil {
		cf.CommentRoot = map[int32]int32{}
	}
	cf.CommentRoot[postID] = rootID
	if err := s.saveFeatures(ctx, ch.ID, cf); err != nil {
		return
	}
	gf := s.features(ctx, group.ID)
	if gf.CommentByRoot == nil {
		gf.CommentByRoot = map[int32]CommentLink{}
	}
	gf.CommentByRoot[rootID] = CommentLink{ChannelID: ch.ID, MsgID: postID}
	if gf.LinkedChatID == 0 {
		gf.LinkedChatID = ch.ID
	}
	_ = s.saveFeatures(ctx, group.ID, gf)
	s.emit(ctx, 0, 0, group.ID, func(viewer int64) *mtproto.Updates {
		copied, err := s.store.GetMessage(ctx, group.ID, rootID)
		if err != nil || copied == nil {
			return nil
		}
		view, err := unmarshalMessage(copied.Data)
		if err != nil {
			return nil
		}
		return s.updatesWith(ctx, viewer, group, newMessageUpdate(viewMessage(view, 0, viewer), groupPts, 1))
	})
}

func stripReplies(ctx context.Context, s *Service, channelID int64, postID int32, body *mtproto.Message) {
	if body == nil || body.Replies == nil {
		return
	}
	body.Replies = nil
	raw, err := proto.Marshal(body)
	if err != nil {
		return
	}
	_ = s.store.ReplaceMessageBody(ctx, channelID, postID, raw)
}

func (s *Service) linkedDiscussion(ctx context.Context, ch *Channel, msg *mtproto.Message) *Channel {
	if !commentsOn() || ch == nil || !ch.Broadcast || isServiceMessage(msg) {
		return nil
	}
	id := s.features(ctx, ch.ID).LinkedChatID
	if id == 0 {
		return nil
	}
	group, err := s.reload(ctx, id)
	if err != nil || group == nil || !group.Megagroup || group.Deleted {
		return nil
	}
	return group
}

func (s *Service) noteComment(ctx context.Context, groupID int64, rootID, commentID, groupPts int32, sender int64) error {
	gf := s.features(ctx, groupID)
	if gf.CommentMsgRoot[commentID] == rootID {
		return nil
	}
	link, ok := gf.CommentByRoot[rootID]
	if !ok {
		return nil
	}
	stored, err := s.store.GetMessage(ctx, link.ChannelID, link.MsgID)
	if err != nil {
		return err
	}
	if stored == nil || stored.Deleted {
		return nil
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return err
	}
	replies := body.GetReplies()
	count := int32(0)
	var recent []*mtproto.Peer
	if replies != nil {
		count = replies.GetReplies()
		recent = replies.GetRecentRepliers()
	}
	body.Replies = commentReplies(groupID, groupPts, count+1, pushReplier(recent, sender), commentID)
	raw, err := proto.Marshal(body)
	if err != nil {
		return err
	}
	if err := s.store.ReplaceMessageBody(ctx, link.ChannelID, link.MsgID, raw); err != nil {
		return err
	}
	gf = s.features(ctx, groupID)
	if gf.CommentMsgRoot == nil {
		gf.CommentMsgRoot = map[int32]int32{}
	}
	gf.CommentMsgRoot[commentID] = rootID
	return s.saveFeatures(ctx, groupID, gf)
}

func commentReplies(groupID int64, pts, count int32, recent []*mtproto.Peer, maxID int32) *mtproto.MessageReplies {
	out := &mtproto.MessageReplies{
		Comments:   true,
		Replies:    count,
		RepliesPts: pts,
		ChannelId:  wrapperspb.Int64(groupID),
	}
	if len(recent) > 0 {
		out.RecentRepliers = recent
	}
	if maxID > 0 {
		out.MaxId = wrapperspb.Int32(maxID)
	}
	return mtproto.MakeTLMessageReplies(out).To_MessageReplies()
}

func pushReplier(list []*mtproto.Peer, user int64) []*mtproto.Peer {
	if user == 0 {
		return list
	}
	out := []*mtproto.Peer{mtproto.MakePeerUser(user)}
	for _, p := range list {
		if p.GetUserId() == user {
			continue
		}
		out = append(out, p)
		if len(out) == 3 {
			break
		}
	}
	return out
}

func isServiceMessage(msg *mtproto.Message) bool {
	return msg == nil || msg.GetPredicateName() == mtproto.Predicate_messageService || msg.GetAction() != nil
}

func replyMsgID(msg *mtproto.Message) int32 {
	h := msg.GetReplyTo()
	if h == nil {
		return 0
	}
	if id := h.GetReplyToMsgId(); id != 0 {
		return id
	}
	if id := h.GetReplyToMsgId_INT32(); id != 0 {
		return id
	}
	return h.GetReplyToMsgId_FLAGINT32().GetValue()
}

func replyTopID(msg *mtproto.Message) int32 {
	h := msg.GetReplyTo()
	if h == nil {
		return 0
	}
	return h.GetReplyToTopId().GetValue()
}

type commentThread struct {
	GroupID   int64
	RootID    int32
	ChannelID int64
	PostID    int32
}

func (s *Service) findCommentThread(ctx context.Context, channelID int64, msgID int32) (commentThread, bool) {
	if !commentsOn() || msgID == 0 {
		return commentThread{}, false
	}
	f := s.features(ctx, channelID)
	if root, ok := f.CommentRoot[msgID]; ok && f.LinkedChatID != 0 {
		return commentThread{GroupID: f.LinkedChatID, RootID: root, ChannelID: channelID, PostID: msgID}, true
	}
	if link, ok := f.CommentByRoot[msgID]; ok {
		return commentThread{GroupID: channelID, RootID: msgID, ChannelID: link.ChannelID, PostID: link.MsgID}, true
	}
	if root := f.CommentMsgRoot[msgID]; root != 0 {
		if link, ok := f.CommentByRoot[root]; ok {
			return commentThread{GroupID: channelID, RootID: root, ChannelID: link.ChannelID, PostID: link.MsgID}, true
		}
	}
	return commentThread{}, false
}

func (s *Service) allowCommentRead(ctx context.Context, self int64, th commentThread) error {
	if err := s.canViewChannel(ctx, th.ChannelID, self); err == nil {
		return nil
	} else if err != mtproto.ErrChannelPrivate && err != mtproto.ErrUserBannedInChannel {
		return err
	}
	p, gerr := s.store.GetParticipant(ctx, th.GroupID, self)
	if gerr != nil {
		return gerr
	}
	if p != nil && p.InChannel() {
		return nil
	}
	return s.canViewChannel(ctx, th.ChannelID, self)
}

func (s *Service) commentMessages(ctx context.Context, self int64, th commentThread) (root *mtproto.Message, comments []*mtproto.Message, err error) {
	stored, err := s.store.GetMessage(ctx, th.GroupID, th.RootID)
	if err != nil {
		return nil, nil, err
	}
	if stored == nil || stored.Deleted {
		return nil, nil, mtproto.ErrMsgIdInvalid
	}
	body, err := unmarshalMessage(stored.Data)
	if err != nil {
		return nil, nil, err
	}
	root = viewMessage(body, stored.SenderID, self)
	all, err := s.store.ListMessages(ctx, th.GroupID)
	if err != nil {
		return nil, nil, err
	}
	f := s.features(ctx, th.GroupID)
	for _, row := range all {
		if row == nil || row.Deleted || row.ID == th.RootID {
			continue
		}
		if f.CommentMsgRoot[row.ID] != th.RootID {
			continue
		}
		one, err := unmarshalMessage(row.Data)
		if err != nil {
			continue
		}
		comments = append(comments, viewMessage(one, row.SenderID, self))
	}
	sortMessages(comments)
	return root, comments, nil
}

func sortMessages(msgs []*mtproto.Message) {
	for i := 0; i < len(msgs); i++ {
		for j := i + 1; j < len(msgs); j++ {
			if msgs[j].GetId() < msgs[i].GetId() {
				msgs[i], msgs[j] = msgs[j], msgs[i]
			}
		}
	}
}

func (s *Service) discussionChats(ctx context.Context, self int64, th commentThread) []*mtproto.Chat {
	var out []*mtproto.Chat
	if ch, err := s.reload(ctx, th.ChannelID); err == nil && ch != nil {
		out = append(out, s.discussionChat(ctx, self, ch))
	}
	if th.GroupID != th.ChannelID {
		if g, err := s.reload(ctx, th.GroupID); err == nil && g != nil {
			out = append(out, s.discussionChat(ctx, self, g))
		}
	}
	if out == nil {
		out = []*mtproto.Chat{}
	}
	return out
}

// discussionChat is the chat object returned with a comment thread. A viewer
// who can see the channel but has not joined the discussion group still
// receives the group (left=true) and its access hash, which is what official
// clients use to send the comment.
func (s *Service) discussionChat(ctx context.Context, viewer int64, ch *Channel) *mtproto.Chat {
	p, _ := s.store.GetParticipant(ctx, ch.ID, viewer)
	if (p != nil && p.InChannel()) || ch.Public() {
		return s.chatFor(ctx, viewer, ch, p)
	}
	feat := s.features(ctx, ch.ID)
	return mtproto.MakeTLChannel(&mtproto.Chat{
		Left:                        true,
		Broadcast:                   ch.Broadcast,
		Megagroup:                   ch.Megagroup,
		Forum:                       ch.Forum,
		Id:                          ch.ID,
		AccessHash_FLAGINT64:        wrapperspb.Int64(ch.AccessHash),
		Title:                       ch.Title,
		Date:                        ch.Date,
		ParticipantsCount_FLAGINT32: wrapperspb.Int32(ch.ParticipantsCount),
		JoinRequest:                 feat.JoinRequest,
	}).To_Chat()
}

func (s *Service) repliesFromComments(ctx context.Context, self, channelID int64, msgID int32, q HistoryQuery) (*mtproto.Messages_Messages, bool, error) {
	th, ok := s.findCommentThread(ctx, channelID, msgID)
	if !ok {
		return nil, false, nil
	}
	if err := s.allowCommentRead(ctx, self, th); err != nil {
		return nil, true, err
	}
	_, comments, err := s.commentMessages(ctx, self, th)
	if err != nil {
		return nil, true, err
	}
	total := int32(len(comments))
	page := pageDescAt(comments, q.OffsetID, q.OffsetDate, q.AddOffset, q.Limit)
	peer, err := s.reload(ctx, channelID)
	pts := int32(0)
	if err == nil && peer != nil {
		pts = peer.Pts
	}
	return mtproto.MakeTLMessagesChannelMessages(&mtproto.Messages_Messages{
		Pts:      pts,
		Count:    total,
		Messages: nonempty(page),
		Chats:    s.discussionChats(ctx, self, th),
		Users:    []*mtproto.User{},
	}).To_Messages_Messages(), true, nil
}

func (s *Service) discussionFromComments(ctx context.Context, self, channelID int64, msgID int32) (*mtproto.Messages_DiscussionMessage, bool, error) {
	th, ok := s.findCommentThread(ctx, channelID, msgID)
	if !ok {
		return nil, false, nil
	}
	if err := s.allowCommentRead(ctx, self, th); err != nil {
		return nil, true, err
	}
	root, comments, err := s.commentMessages(ctx, self, th)
	if err != nil {
		return nil, true, err
	}
	msgs := []*mtproto.Message{root}
	if len(comments) > 20 {
		comments = comments[len(comments)-20:]
	}
	msgs = append(msgs, comments...)
	if post, err := s.store.GetMessage(ctx, th.ChannelID, th.PostID); err == nil && post != nil && !post.Deleted {
		if body, err := unmarshalMessage(post.Data); err == nil {
			msgs = append(msgs, viewMessage(body, post.SenderID, self))
		}
	}
	maxID := root.GetId()
	for _, m := range comments {
		if m.GetId() > maxID {
			maxID = m.GetId()
		}
	}
	readInbox := maxID
	unread := int32(0)
	if st, _ := s.store.GetRead(ctx, th.GroupID, self); st != nil && st.ReadInboxMaxID > 0 {
		readInbox = st.ReadInboxMaxID
		for _, m := range comments {
			if m.GetId() > readInbox {
				unread++
			}
		}
	}
	return mtproto.MakeTLMessagesDiscussionMessage(&mtproto.Messages_DiscussionMessage{
		Messages:       msgs,
		MaxId:          wrapperspb.Int32(maxID),
		ReadInboxMaxId: wrapperspb.Int32(readInbox),
		UnreadCount:    unread,
		Chats:          s.discussionChats(ctx, self, th),
		Users:          []*mtproto.User{},
	}).To_Messages_DiscussionMessage(), true, nil
}
