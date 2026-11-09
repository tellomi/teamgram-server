package channel

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// SearchPosts is channels.searchPosts.
//
// It covers public broadcast channels (a username is set) and every channel
// the user has joined. Deleted, banned, and restricted channels are left out,
// and so is a channel that banned or kicked the user. The text match is a
// SQL LIKE '%q%' with % and _ escaped. Rows are newest first. offset_rate,
// offset_peer, and offset_id are the previous page's date, channel, and
// message id.
func (s *Service) SearchPosts(ctx context.Context, self int64, q PostQuery) (*mtproto.Messages_Messages, error) {
	needle, err := postNeedle(q.Needle)
	if err != nil {
		return nil, err
	}
	limit := saneLimit(q.Limit)
	q.Needle = needle
	q.Limit = limit + 1
	rows, err := s.store.SearchPosts(ctx, self, q)
	if err != nil {
		return nil, err
	}
	inexact := int32(len(rows)) > limit
	if inexact {
		rows = rows[:limit]
	}
	msgs := make([]*mtproto.Message, 0, len(rows))
	var chats []*mtproto.Chat
	var senders []int64
	seen := map[int64]bool{}
	for _, stored := range rows {
		if stored == nil {
			continue
		}
		body, err := unmarshalMessage(stored.Data)
		if err != nil || body == nil {
			continue
		}
		channelID := stored.ChannelID
		if id := body.GetPeerId().GetChannelId(); id != 0 {
			channelID = id
		}
		if body.GetPeerId().GetChannelId() == 0 && channelID != 0 {
			body.PeerId = mtproto.MakePeerChannel(channelID)
		}
		if body.PredicateName == "" {
			body.PredicateName = mtproto.Predicate_message
		}
		if body.GetMedia() != nil && body.GetMedia().GetPredicateName() == "" {
			body.Media = nil
		}
		sender := stored.SenderID
		if id := body.GetFromId().GetUserId(); id != 0 {
			sender = id
		}
		var ch *Channel
		if channelID != 0 {
			ch, _ = s.store.GetChannel(ctx, channelID)
		}
		if sender == 0 && ch != nil {
			sender = ch.CreatorID
		}
		if sender == 0 {
			sender = self
		}
		if body.GetFromId().GetUserId() == 0 && !chIsBroadcast(ch) {
			body.FromId = mtproto.MakePeerUser(sender)
		}
		msgs = append(msgs, viewMessage(body, sender, self))
		senders = append(senders, sender)
		if channelID != 0 && !seen[channelID] {
			seen[channelID] = true
			chats = append(chats, postSearchChat(s, ctx, self, ch, channelID))
		}
	}
	if chats == nil {
		chats = []*mtproto.Chat{}
	}
	senders = append(senders, self)
	out := mtproto.MakeTLMessagesMessagesSlice(&mtproto.Messages_Messages{
		Inexact:  inexact,
		Count:    int32(len(msgs)),
		Messages: msgs,
		Topics:   []*mtproto.ForumTopic{},
		Chats:    chats,
		Users:    channelStubUsers(self, senders),
	}).To_Messages_Messages()
	if inexact {
		out.Count = limit + 1
	}
	if n := len(msgs); n > 0 {
		out.NextRate = &wrapperspb.Int32Value{Value: msgs[n-1].GetDate()}
	}
	return out, nil
}

// SearchPublic matches public channel and group titles and usernames.
func (s *Service) SearchPublic(ctx context.Context, self int64, q string, limit int32) ([]*Channel, error) {
	q = strings.TrimSpace(strings.TrimPrefix(q, "@"))
	if q == "" {
		return nil, nil
	}
	if limit <= 0 || limit > 50 {
		limit = 50
	}
	return s.store.SearchPublic(ctx, self, q, limit)
}

// Hidden reports that a channel must not appear in search.
func (s *Service) Hidden(ctx context.Context, id int64) bool {
	if s == nil || s.store == nil || id == 0 {
		return false
	}
	ch, err := s.store.GetChannel(ctx, id)
	if err != nil || ch == nil {
		return false
	}
	return channelHidden(ch)
}

func chIsBroadcast(ch *Channel) bool {
	return ch != nil && ch.Broadcast && !ch.Megagroup
}

func postSearchChat(s *Service, ctx context.Context, self int64, ch *Channel, channelID int64) *mtproto.Chat {
	if ch != nil && s != nil {
		if chat := s.ChatFor(ctx, self, ch); chat != nil {
			return chat
		}
	}
	return mtproto.MakeTLChannel(&mtproto.Chat{
		Broadcast: true,
		Id:        channelID,
		Title:     "",
		Photo:     mtproto.MakeTLChatPhotoEmpty(nil).To_ChatPhoto(),
	}).To_Chat()
}

func postNeedle(raw string) (string, error) {
	needle := strings.TrimSpace(raw)
	needle = strings.TrimPrefix(needle, "#")
	needle = strings.TrimSpace(needle)
	if needle == "" || !utf8.ValidString(needle) {
		return "", mtproto.ErrSearchQueryEmpty
	}
	return needle, nil
}

func saneLimit(limit int32) int32 {
	if limit <= 0 {
		return 20
	}
	if limit > 50 {
		return 50
	}
	return limit
}

// likeContains is a LIKE pattern that matches needle anywhere.
// %, _, and \ in the needle are literals.
func likeContains(needle string) string {
	var b strings.Builder
	b.Grow(len(needle) + 2)
	b.WriteByte('%')
	for _, r := range needle {
		switch r {
		case '\\', '%', '_':
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('%')
	return b.String()
}

func containsFold(text, needle string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(needle))
}

func channelHidden(ch *Channel) bool {
	return ch == nil || ch.Deleted || ch.Restricted || ch.Banned
}

func viewerBlocked(p *Participant) bool {
	if p == nil {
		return false
	}
	return p.State == StateKicked || p.Role == RoleBanned
}

func postSearchVisible(ch *Channel, p *Participant) bool {
	if channelHidden(ch) || viewerBlocked(p) {
		return false
	}
	if ch.Broadcast && ch.Username != "" {
		return true
	}
	return p != nil && p.State == StateNormal && p.Role != RoleBanned
}

func olderPost(date int32, channelID int64, msgID int32, q PostQuery) bool {
	if q.OffsetRate <= 0 && q.OffsetID <= 0 {
		return true
	}
	if q.OffsetRate <= 0 {
		return msgID < q.OffsetID
	}
	if date != q.OffsetRate {
		return date < q.OffsetRate
	}
	if channelID != q.OffsetPeer {
		return channelID < q.OffsetPeer
	}
	return msgID < q.OffsetID
}

func messagePlain(data []byte) string {
	msg, err := unmarshalMessage(data)
	if err != nil || msg == nil {
		return ""
	}
	return msg.GetMessage()
}
