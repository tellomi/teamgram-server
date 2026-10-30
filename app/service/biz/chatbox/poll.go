package chatbox

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const maxPollAnswers = 10

type voteRec struct {
	UserID  int64    `json:"user_id"`
	Options [][]byte `json:"options"`
	Date    int32    `json:"date"`
}

type pollDoc struct {
	ID          int64     `json:"id"`
	PeerType    int32     `json:"peer_type"`
	PeerID      int64     `json:"peer_id"`
	MsgID       int32     `json:"msg_id"`
	Creator     int64     `json:"creator"`
	Poll        []byte    `json:"poll"`
	Correct     [][]byte  `json:"correct,omitempty"`
	Solution    string    `json:"solution,omitempty"`
	SolutionEnt []byte    `json:"solution_ent,omitempty"`
	Votes       []voteRec `json:"votes,omitempty"`
	Unread      []int64   `json:"unread,omitempty"`
}

func pollKey(id int64) string { return "poll:" + strconv.FormatInt(id, 10) }

func pollIndexKey(peerType int32, peerID int64, msgID int32) string {
	return "pollidx:" + peerKey(peerType, peerID) + ":" + strconv.FormatInt(int64(msgID), 10)
}

func pollPeerKey(peerType int32, peerID int64) string {
	return "pollpeer:" + peerKey(peerType, peerID)
}

// CreatePoll validates an outgoing poll and remembers it under its id.
// The message id is attached later with BindPoll.
func (b *Book) CreatePoll(ctx context.Context, creator int64, poll *mtproto.Poll, correct [][]byte, solution string, solutionEnt []*mtproto.MessageEntity) (*mtproto.MessageMedia, error) {
	doc, err := normalizePoll(creator, poll, correct, solution, solutionEnt)
	if err != nil {
		return nil, err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.putPoll(ctx, doc); err != nil {
		return nil, err
	}
	return b.mediaLocked(doc, creator, false), nil
}

// ObserveSent links polls inside a successful send to their new message ids.
func (b *Book) ObserveSent(ctx context.Context, peer *mtproto.PeerUtil, ups *mtproto.Updates) {
	if b == nil || peer == nil || ups == nil {
		return
	}
	for _, u := range ups.Updates {
		msg := u.GetMessage_MESSAGE()
		if msg == nil || msg.GetMedia() == nil || msg.GetMedia().GetPoll() == nil {
			continue
		}
		b.BindPoll(ctx, peer.PeerType, peer.PeerId, msg.GetId(), msg.GetMedia().GetPoll().GetId())
	}
}

// BindPoll attaches a sent message id to a poll created just before the send.
func (b *Book) BindPoll(ctx context.Context, peerType int32, peerID int64, msgID int32, pollID int64) {
	if pollID == 0 || msgID == 0 {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.getPoll(ctx, pollID)
	if err != nil || doc == nil {
		return
	}
	doc.PeerType = peerType
	doc.PeerID = peerID
	doc.MsgID = msgID
	_ = b.putPoll(ctx, doc)
	idx := pollIndexKey(peerType, peerID, msgID)
	b.pollIdx[idx] = pollID
	_ = b.save(ctx, idx, pollID)
	b.addPeerPoll(ctx, peerType, peerID, pollID)
}

// EnsurePoll creates a poll record from a message that already carries one
// (for example a channel message loaded after a restart).
func (b *Book) EnsurePoll(ctx context.Context, creator int64, peerType int32, peerID int64, msgID int32, poll *mtproto.Poll) error {
	if poll == nil || poll.GetId() == 0 {
		return mtproto.ErrMediaInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if doc, err := b.getPoll(ctx, poll.GetId()); err != nil {
		return err
	} else if doc != nil {
		if doc.MsgID == 0 && msgID != 0 {
			doc.PeerType, doc.PeerID, doc.MsgID = peerType, peerID, msgID
			if err := b.putPoll(ctx, doc); err != nil {
				return err
			}
			b.addPeerPoll(ctx, peerType, peerID, doc.ID)
		}
		return nil
	}
	doc, err := normalizePoll(creator, poll, nil, "", nil)
	if err != nil {
		return err
	}
	doc.PeerType, doc.PeerID, doc.MsgID = peerType, peerID, msgID
	if err := b.putPoll(ctx, doc); err != nil {
		return err
	}
	b.addPeerPoll(ctx, peerType, peerID, doc.ID)
	return nil
}

// Vote records the caller's choice and returns updateMessagePoll.
// An empty option list retracts the vote when the poll allows it.
func (b *Book) Vote(ctx context.Context, voter int64, peerType int32, peerID int64, msgID int32, pollID int64, options [][]byte) (*mtproto.Updates, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.pollForMessage(ctx, peerType, peerID, msgID, pollID)
	if err != nil {
		return nil, err
	}
	if err := b.applyVote(doc, voter, options); err != nil {
		return nil, err
	}
	if err := b.putPoll(ctx, doc); err != nil {
		return nil, err
	}
	return b.pollUpdate(doc, voter), nil
}

// PollResultsUpdate returns updateMessagePoll, or an empty updates when hash matches.
func (b *Book) PollResultsUpdate(ctx context.Context, viewer int64, peerType int32, peerID int64, msgID int32, pollID, hash int64) (*mtproto.Updates, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.pollForMessage(ctx, peerType, peerID, msgID, pollID)
	if err != nil {
		return nil, err
	}
	if hash != 0 && hash == doc.hash() {
		return mtproto.MakeEmptyUpdates(), nil
	}
	return b.pollUpdate(doc, viewer), nil
}

// PollVotes lists who chose an option. option nil lists every voter.
func (b *Book) PollVotes(ctx context.Context, viewer int64, peerType int32, peerID int64, msgID int32, pollID int64, option []byte, offset string, limit int32) (*mtproto.Messages_VotesList, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.pollForMessage(ctx, peerType, peerID, msgID, pollID)
	if err != nil {
		return nil, err
	}
	poll := doc.poll()
	if poll == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if len(option) > 0 && !doc.hasOption(option) {
		return nil, mtproto.ErrOptionInvalid
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	start := 0
	if offset != "" {
		n, ok := atoi32(offset)
		if !ok || n < 0 {
			return nil, mtproto.ErrOffsetInvalid
		}
		start = n
	}
	var votes []*mtproto.MessagePeerVote
	var users []*mtproto.User
	seen := map[int64]bool{}
	for _, v := range doc.Votes {
		if len(option) > 0 && !hasBytes(v.Options, option) {
			continue
		}
		peer := mtproto.MakePeerUser(v.UserID)
		if poll.MultipleChoice || len(v.Options) > 1 {
			votes = append(votes, mtproto.MakeTLMessagePeerVoteMultiple(&mtproto.MessagePeerVote{
				Peer:    peer,
				Options: v.Options,
				Date:    v.Date,
			}).To_MessagePeerVote())
		} else {
			var op []byte
			if len(v.Options) > 0 {
				op = v.Options[0]
			}
			votes = append(votes, mtproto.MakeTLMessagePeerVote(&mtproto.MessagePeerVote{
				Peer:   peer,
				Option: op,
				Date:   v.Date,
			}).To_MessagePeerVote())
		}
		if !seen[v.UserID] {
			seen[v.UserID] = true
			users = append(users, mtproto.MakeTLUser(&mtproto.User{Id: v.UserID}).To_User())
		}
	}
	_ = viewer
	if start > len(votes) {
		start = len(votes)
	}
	end := start + int(limit)
	var next *wrapperspb.StringValue
	if end < len(votes) {
		next = &wrapperspb.StringValue{Value: strconv.Itoa(end)}
	} else {
		end = len(votes)
	}
	slice := votes[start:end]
	if slice == nil {
		slice = []*mtproto.MessagePeerVote{}
	}
	if users == nil {
		users = []*mtproto.User{}
	}
	return mtproto.MakeTLMessagesVotesList(&mtproto.Messages_VotesList{
		Count:                       int32(len(votes)),
		Votes_VECTORMESSAGEPEERVOTE: slice,
		Chats:                       []*mtproto.Chat{},
		Users:                       users,
		NextOffset:                  next,
	}).To_Messages_VotesList(), nil
}

// AddPollAnswer appends an option on an open poll.
func (b *Book) AddPollAnswer(ctx context.Context, voter int64, peerType int32, peerID int64, msgID int32, pollID int64, answer *mtproto.PollAnswer) (*mtproto.Updates, error) {
	if answer == nil || answerText(answer) == "" {
		return nil, mtproto.ErrPollAnswerInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.pollForMessage(ctx, peerType, peerID, msgID, pollID)
	if err != nil {
		return nil, err
	}
	poll := doc.poll()
	if poll == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if doc.closed(int32(b.now().Unix())) {
		return nil, mtproto.ErrMessagePollClosed
	}
	if !poll.OpenAnswers {
		return nil, mtproto.ErrPollAnswerInvalid
	}
	if len(poll.Answers) >= maxPollAnswers {
		return nil, mtproto.ErrPollAnswersInvalid
	}
	text := answerText(answer)
	opt := answer.Option
	if len(opt) == 0 {
		opt = []byte(strconv.Itoa(len(poll.Answers)))
	}
	for _, a := range poll.Answers {
		if answerText(a) == text || bytes.Equal(a.Option, opt) {
			return nil, mtproto.ErrPollOptionDuplicate
		}
	}
	poll.Answers = append(poll.Answers, normalizeAnswer(answer, opt))
	if err := doc.setPoll(poll); err != nil {
		return nil, err
	}
	if err := b.putPoll(ctx, doc); err != nil {
		return nil, err
	}
	return b.pollUpdate(doc, voter), nil
}

// DeletePollAnswer removes an option from an open poll.
func (b *Book) DeletePollAnswer(ctx context.Context, voter int64, peerType int32, peerID int64, msgID int32, pollID int64, option []byte) (*mtproto.Updates, error) {
	if len(option) == 0 {
		return nil, mtproto.ErrOptionInvalid
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.pollForMessage(ctx, peerType, peerID, msgID, pollID)
	if err != nil {
		return nil, err
	}
	poll := doc.poll()
	if poll == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	if doc.closed(int32(b.now().Unix())) {
		return nil, mtproto.ErrMessagePollClosed
	}
	if !poll.OpenAnswers {
		return nil, mtproto.ErrPollAnswerInvalid
	}
	found := false
	next := poll.Answers[:0]
	for _, a := range poll.Answers {
		if bytes.Equal(a.Option, option) {
			found = true
			continue
		}
		next = append(next, a)
	}
	if !found {
		return nil, mtproto.ErrOptionInvalid
	}
	if len(next) == 0 {
		return nil, mtproto.ErrPollAnswersInvalid
	}
	poll.Answers = next
	if err := doc.setPoll(poll); err != nil {
		return nil, err
	}
	var votes []voteRec
	for _, v := range doc.Votes {
		kept := v.Options[:0]
		for _, op := range v.Options {
			if !bytes.Equal(op, option) {
				kept = append(kept, op)
			}
		}
		if len(kept) == 0 {
			continue
		}
		v.Options = kept
		votes = append(votes, v)
	}
	doc.Votes = votes
	if err := b.putPoll(ctx, doc); err != nil {
		return nil, err
	}
	return b.pollUpdate(doc, voter), nil
}

// UnreadPollMessages returns messages whose poll has votes the viewer has not read.
func (b *Book) UnreadPollMessages(ctx context.Context, viewer int64, peerType int32, peerID int64, offsetID, addOffset, limit, maxID, minID int32) ([]*mtproto.Message, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids, err := b.peerPolls(ctx, peerType, peerID)
	if err != nil {
		return nil, err
	}
	var msgs []*mtproto.Message
	for _, id := range ids {
		doc, err := b.getPoll(ctx, id)
		if err != nil || doc == nil || doc.MsgID == 0 || !doc.unread(viewer) {
			continue
		}
		if maxID > 0 && doc.MsgID >= maxID {
			continue
		}
		if minID > 0 && doc.MsgID <= minID {
			continue
		}
		msgs = append(msgs, doc.message(viewer))
	}
	return paginateMessages(msgs, offsetID, addOffset, limit), nil
}

// ReadPollVotes clears the unread-vote flag for this viewer in the dialog.
func (b *Book) ReadPollVotes(ctx context.Context, viewer int64, peerType int32, peerID int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	ids, err := b.peerPolls(ctx, peerType, peerID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		doc, err := b.getPoll(ctx, id)
		if err != nil || doc == nil {
			continue
		}
		if !doc.clearUnread(viewer) {
			continue
		}
		if err := b.putPoll(ctx, doc); err != nil {
			return err
		}
	}
	return nil
}

// PublicResults is the poll media to store on the message (no per-user chosen bit).
func (b *Book) PublicResults(ctx context.Context, pollID int64) (*mtproto.Poll, *mtproto.PollResults) {
	b.mu.Lock()
	defer b.mu.Unlock()
	doc, err := b.getPoll(ctx, pollID)
	if err != nil || doc == nil {
		return nil, nil
	}
	return doc.poll(), doc.results(0)
}

func (b *Book) applyVote(doc *pollDoc, voter int64, options [][]byte) error {
	now := int32(b.now().Unix())
	if doc.closed(now) {
		return mtproto.ErrMessagePollClosed
	}
	poll := doc.poll()
	if poll == nil {
		return mtproto.ErrMessageIdInvalid
	}
	if !poll.MultipleChoice && len(options) > 1 {
		return mtproto.ErrOptionInvalid
	}
	seen := map[string]bool{}
	for _, op := range options {
		if len(op) == 0 || !doc.hasOption(op) {
			return mtproto.ErrPollOptionInvalid
		}
		if seen[string(op)] {
			return mtproto.ErrPollOptionDuplicate
		}
		seen[string(op)] = true
	}
	prev := doc.voteOf(voter)
	if len(options) == 0 {
		if prev == nil {
			return nil
		}
		if poll.RevotingDisabled || poll.Quiz {
			return mtproto.ErrRevoteNotAllowed
		}
		doc.removeVote(voter)
		return nil
	}
	if prev != nil && (poll.RevotingDisabled || poll.Quiz) && !sameOptions(prev.Options, options) {
		return mtproto.ErrRevoteNotAllowed
	}
	doc.removeVote(voter)
	copied := make([][]byte, len(options))
	for i, op := range options {
		copied[i] = append([]byte(nil), op...)
	}
	doc.Votes = append(doc.Votes, voteRec{UserID: voter, Options: copied, Date: now})
	if voter != doc.Creator {
		doc.markUnread(doc.Creator)
	}
	return nil
}

func (b *Book) pollUpdate(doc *pollDoc, viewer int64) *mtproto.Updates {
	poll := doc.poll()
	if poll != nil {
		poll.Creator = viewer == doc.Creator
		poll.Hash = doc.hash()
	}
	up := mtproto.MakeTLUpdateMessagePoll(&mtproto.Update{
		PollId:      doc.ID,
		Peer_PEER:   mtproto.MakePeer(doc.PeerType, doc.PeerID),
		MsgId_INT32: doc.MsgID,
		Poll:        poll,
		Results:     doc.results(viewer),
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(up)
}

func (b *Book) mediaLocked(doc *pollDoc, viewer int64, chosen bool) *mtproto.MessageMedia {
	poll := doc.poll()
	if poll != nil {
		poll.Creator = viewer == doc.Creator
	}
	var res *mtproto.PollResults
	if chosen {
		res = doc.results(viewer)
	} else {
		res = doc.results(0)
	}
	return mtproto.MakeTLMessageMediaPoll(&mtproto.MessageMedia{
		Poll:    poll,
		Results: res,
	}).To_MessageMedia()
}

func (b *Book) pollForMessage(ctx context.Context, peerType int32, peerID int64, msgID int32, pollID int64) (*pollDoc, error) {
	if pollID == 0 && msgID != 0 {
		idx := pollIndexKey(peerType, peerID, msgID)
		var id int64
		if v, ok := b.pollIdx[idx]; ok {
			id = v
		}
		var stored int64
		if err := b.load(ctx, idx, &stored); err != nil {
			return nil, err
		} else if stored != 0 {
			id = stored
			b.pollIdx[idx] = id
		}
		pollID = id
	}
	if pollID == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	doc, err := b.getPoll(ctx, pollID)
	if err != nil {
		return nil, err
	}
	if doc == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return doc, nil
}

func (b *Book) getPoll(ctx context.Context, id int64) (*pollDoc, error) {
	if doc := b.polls[id]; doc != nil && b.db == nil {
		return doc, nil
	}
	doc := &pollDoc{}
	key := pollKey(id)
	if b.db != nil {
		raw, ok, err := b.db.Load(ctx, key)
		if err != nil {
			return nil, err
		}
		if !ok {
			if b.polls[id] != nil {
				return b.polls[id], nil
			}
			return nil, nil
		}
		if err := json.Unmarshal(raw, doc); err != nil {
			return nil, err
		}
		b.polls[id] = doc
		return doc, nil
	}
	return b.polls[id], nil
}

func (b *Book) putPoll(ctx context.Context, doc *pollDoc) error {
	b.polls[doc.ID] = doc
	if doc.MsgID != 0 {
		idx := pollIndexKey(doc.PeerType, doc.PeerID, doc.MsgID)
		b.pollIdx[idx] = doc.ID
		if err := b.save(ctx, idx, doc.ID); err != nil {
			return err
		}
	}
	return b.save(ctx, pollKey(doc.ID), doc)
}

func (b *Book) addPeerPoll(ctx context.Context, peerType int32, peerID, pollID int64) {
	key := pollPeerKey(peerType, peerID)
	ids := b.pollPeer[key]
	var stored []int64
	_ = b.load(ctx, key, &stored)
	if len(stored) > 0 {
		ids = stored
	}
	for _, id := range ids {
		if id == pollID {
			b.pollPeer[key] = ids
			return
		}
	}
	ids = append(ids, pollID)
	b.pollPeer[key] = ids
	_ = b.save(ctx, key, ids)
}

func (b *Book) peerPolls(ctx context.Context, peerType int32, peerID int64) ([]int64, error) {
	key := pollPeerKey(peerType, peerID)
	var ids []int64
	if err := b.load(ctx, key, &ids); err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		ids = b.pollPeer[key]
	}
	b.pollPeer[key] = ids
	return ids, nil
}

func normalizePoll(creator int64, poll *mtproto.Poll, correct [][]byte, solution string, solutionEnt []*mtproto.MessageEntity) (*pollDoc, error) {
	if poll == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	q := pollQuestion(poll)
	if q == "" {
		return nil, mtproto.ErrPollQuestionInvalid
	}
	if len(poll.Answers) > maxPollAnswers || len(poll.Answers) == 0 {
		return nil, mtproto.ErrPollAnswersInvalid
	}
	if !poll.OpenAnswers && len(poll.Answers) < 2 {
		return nil, mtproto.ErrPollAnswersInvalid
	}
	cp := proto.Clone(poll).(*mtproto.Poll)
	if cp.Id == 0 {
		cp.Id = randInt63()
	}
	cp.Question = q
	cp.Question_STRING = q
	if cp.Question_TEXTWITHENTITIES == nil {
		cp.Question_TEXTWITHENTITIES = mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Text: q, Entities: []*mtproto.MessageEntity{},
		}).To_TextWithEntities()
	}
	seen := map[string]bool{}
	for i, a := range cp.Answers {
		if answerText(a) == "" {
			return nil, mtproto.ErrPollAnswersInvalid
		}
		opt := a.Option
		if len(opt) == 0 {
			opt = []byte(strconv.Itoa(i))
		}
		if seen[string(opt)] {
			return nil, mtproto.ErrPollOptionDuplicate
		}
		seen[string(opt)] = true
		cp.Answers[i] = normalizeAnswer(a, opt)
	}
	cp = mtproto.MakeTLPoll(cp).To_Poll()
	raw, err := proto.Marshal(cp)
	if err != nil {
		return nil, err
	}
	var ent []byte
	if len(solutionEnt) > 0 {
		shell := &mtproto.PollResults{SolutionEntities: solutionEnt}
		ent, _ = proto.Marshal(shell)
	}
	for _, op := range correct {
		if !seen[string(op)] {
			return nil, mtproto.ErrPollOptionInvalid
		}
	}
	return &pollDoc{
		ID: cp.Id, Creator: creator, Poll: raw, Correct: correct, Solution: solution, SolutionEnt: ent,
	}, nil
}

func normalizeAnswer(a *mtproto.PollAnswer, opt []byte) *mtproto.PollAnswer {
	text := answerText(a)
	out := mtproto.MakePollAnswerWithText(text, append([]byte(nil), opt...))
	return out
}

func (d *pollDoc) poll() *mtproto.Poll {
	if len(d.Poll) == 0 {
		return nil
	}
	p := &mtproto.Poll{}
	if err := proto.Unmarshal(d.Poll, p); err != nil {
		return nil
	}
	return p
}

func (d *pollDoc) setPoll(p *mtproto.Poll) error {
	raw, err := proto.Marshal(p)
	if err != nil {
		return err
	}
	d.Poll = raw
	return nil
}

func (d *pollDoc) closed(now int32) bool {
	p := d.poll()
	if p == nil {
		return true
	}
	if p.Closed {
		return true
	}
	if p.CloseDate != nil && p.CloseDate.Value != 0 && p.CloseDate.Value <= now {
		return true
	}
	return false
}

func (d *pollDoc) hasOption(op []byte) bool {
	p := d.poll()
	if p == nil {
		return false
	}
	for _, a := range p.Answers {
		if bytes.Equal(a.Option, op) {
			return true
		}
	}
	return false
}

func (d *pollDoc) voteOf(user int64) *voteRec {
	for i := range d.Votes {
		if d.Votes[i].UserID == user {
			return &d.Votes[i]
		}
	}
	return nil
}

func (d *pollDoc) removeVote(user int64) {
	out := d.Votes[:0]
	for _, v := range d.Votes {
		if v.UserID != user {
			out = append(out, v)
		}
	}
	d.Votes = out
}

func (d *pollDoc) hash() int64 {
	var h int64 = 1
	h = h*20261 + int64(len(d.Votes))
	p := d.poll()
	if p != nil {
		for _, a := range p.Answers {
			var n int32
			for _, v := range d.Votes {
				if hasBytes(v.Options, a.Option) {
					n++
				}
			}
			h = h*20261 + int64(n)
		}
	}
	if h < 0 {
		h = -h
	}
	return h
}

func (d *pollDoc) results(viewer int64) *mtproto.PollResults {
	p := d.poll()
	if p == nil {
		return mtproto.MakeTLPollResults(&mtproto.PollResults{Results: []*mtproto.PollAnswerVoters{}}).To_PollResults()
	}
	chosen := map[string]bool{}
	if v := d.voteOf(viewer); v != nil && viewer != 0 {
		for _, op := range v.Options {
			chosen[string(op)] = true
		}
	}
	showCorrect := p.Quiz && (viewer == d.Creator || len(chosen) > 0)
	var rows []*mtproto.PollAnswerVoters
	var recent []*mtproto.Peer
	for _, a := range p.Answers {
		var n int32
		for _, v := range d.Votes {
			if hasBytes(v.Options, a.Option) {
				n++
				if p.PublicVoters && len(recent) < 3 {
					recent = append(recent, mtproto.MakePeerUser(v.UserID))
				}
			}
		}
		row := mtproto.MakeTLPollAnswerVoters(&mtproto.PollAnswerVoters{
			Chosen:           chosen[string(a.Option)],
			Correct:          showCorrect && hasBytes(d.Correct, a.Option),
			Option:           a.Option,
			Voters_INT32:     n,
			Voters_FLAGINT32: &wrapperspb.Int32Value{Value: n},
		}).To_PollAnswerVoters()
		rows = append(rows, row)
	}
	if rows == nil {
		rows = []*mtproto.PollAnswerVoters{}
	}
	res := mtproto.MakeTLPollResults(&mtproto.PollResults{
		Results:                     rows,
		TotalVoters:                 &wrapperspb.Int32Value{Value: int32(len(d.Votes))},
		RecentVoters_FLAGVECTORPEER: recent,
		HasUnreadVotes:              viewer != 0 && d.unread(viewer),
	}).To_PollResults()
	if showCorrect && d.Solution != "" && len(chosen) > 0 {
		res.Solution = &wrapperspb.StringValue{Value: d.Solution}
		if len(d.SolutionEnt) > 0 {
			shell := &mtproto.PollResults{}
			if err := proto.Unmarshal(d.SolutionEnt, shell); err == nil {
				res.SolutionEntities = shell.SolutionEntities
			}
		}
	}
	return res
}

func (d *pollDoc) message(viewer int64) *mtproto.Message {
	peer := mtproto.MakePeer(d.PeerType, d.PeerID)
	return mtproto.MakeTLMessage(&mtproto.Message{
		Out:     viewer == d.Creator,
		Id:      d.MsgID,
		FromId:  mtproto.MakePeerUser(d.Creator),
		PeerId:  peer,
		Date:    int32(time.Now().Unix()),
		Message: pollQuestion(d.poll()),
		Media:   (&Book{}).mediaFrom(d, viewer),
	}).To_Message()
}

func (b *Book) mediaFrom(d *pollDoc, viewer int64) *mtproto.MessageMedia {
	return b.mediaLocked(d, viewer, true)
}

func (d *pollDoc) messageMedia(viewer int64) *mtproto.MessageMedia {
	return (&Book{}).mediaLocked(d, viewer, true)
}

func (d *pollDoc) unread(user int64) bool {
	for _, id := range d.Unread {
		if id == user {
			return true
		}
	}
	return false
}

func (d *pollDoc) markUnread(user int64) {
	if user == 0 || d.unread(user) {
		return
	}
	d.Unread = append(d.Unread, user)
}

func (d *pollDoc) clearUnread(user int64) bool {
	out := d.Unread[:0]
	cleared := false
	for _, id := range d.Unread {
		if id == user {
			cleared = true
			continue
		}
		out = append(out, id)
	}
	d.Unread = out
	return cleared
}

func pollQuestion(p *mtproto.Poll) string {
	if p == nil {
		return ""
	}
	if p.Question != "" {
		return p.Question
	}
	if p.Question_STRING != "" {
		return p.Question_STRING
	}
	if p.Question_TEXTWITHENTITIES != nil {
		return p.Question_TEXTWITHENTITIES.Text
	}
	return ""
}

func answerText(a *mtproto.PollAnswer) string {
	if a == nil {
		return ""
	}
	if a.Text != "" {
		return a.Text
	}
	if a.Text_STRING != "" {
		return a.Text_STRING
	}
	if a.Text_TEXTWITHENTITIES != nil {
		return a.Text_TEXTWITHENTITIES.Text
	}
	return ""
}

func hasBytes(list [][]byte, op []byte) bool {
	for _, item := range list {
		if bytes.Equal(item, op) {
			return true
		}
	}
	return false
}

func sameOptions(a, b [][]byte) bool {
	if len(a) != len(b) {
		return false
	}
	for _, item := range a {
		if !hasBytes(b, item) {
			return false
		}
	}
	return true
}

func paginateMessages(msgs []*mtproto.Message, offsetID, addOffset, limit int32) []*mtproto.Message {
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	// Newest first.
	for i := 0; i < len(msgs); i++ {
		for j := i + 1; j < len(msgs); j++ {
			if msgs[j].Id > msgs[i].Id {
				msgs[i], msgs[j] = msgs[j], msgs[i]
			}
		}
	}
	start := 0
	if offsetID > 0 {
		for start < len(msgs) && msgs[start].Id >= offsetID {
			start++
		}
	}
	start += int(addOffset)
	if start < 0 {
		start = 0
	}
	if start > len(msgs) {
		start = len(msgs)
	}
	end := start + int(limit)
	if end > len(msgs) {
		end = len(msgs)
	}
	out := msgs[start:end]
	if out == nil {
		return []*mtproto.Message{}
	}
	return out
}

func randInt63() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return time.Now().UnixNano()
	}
	v := int64(binary.BigEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}
