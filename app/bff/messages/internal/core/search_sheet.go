package core

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/searchbox"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
)

// searchMyMentions is messages.search with inputMessagesFilterMyMentions
// in a private chat or basic group. The mentioned column is set on the
// recipient's copy, so the sender's own mention of someone else is absent.
func (c *MessagesCore) searchMyMentions(peer *mtproto.PeerUtil, q string, offset, limit int32) (*mtproto.Messages_Messages, error) {
	rows, err := c.scanHistory(peer, 20)
	if err != nil {
		c.Logger.Errorf("messages.search - mentions: %v", err)
		return nil, err
	}
	return c.messagesFromRows(keepMentions(rows, q, offset, limit)), nil
}

func keepMentions(rows []searchbox.Row, q string, offset, limit int32) []searchbox.Row {
	q = strings.ToLower(strings.TrimSpace(q))
	if limit <= 0 {
		limit = 20
	}
	var out []searchbox.Row
	for _, row := range rows {
		if !row.Mentioned {
			continue
		}
		if offset > 0 && row.ID >= offset {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(rowText(row)), q) {
			continue
		}
		out = append(out, row)
		if int32(len(out)) >= limit {
			break
		}
	}
	return out
}

// searchSaved is messages.search in Saved Messages with saved_peer_id
// and, when set, saved_reaction. An empty q still filters by the saved peer.
func (c *MessagesCore) searchSaved(peer *mtproto.PeerUtil, in *mtproto.TLMessagesSearch, saved *mtproto.PeerUtil, offset, limit int32) (*mtproto.Messages_Messages, error) {
	rows, err := c.scanHistory(peer, 20)
	if err != nil {
		c.Logger.Errorf("messages.search - saved: %v", err)
		return nil, err
	}
	kept := keepSaved(rows, savedQuery{
		Q:        in.GetQ(),
		MinDate:  in.GetMinDate(),
		MaxDate:  in.GetMaxDate(),
		Offset:   offset,
		Limit:    limit,
		Saved:    saved,
		Reaction: in.GetSavedReaction(),
		Filter:   in.GetFilter(),
	}, func(id int32) bool {
		return chatbox.Shared().HasReaction(c.ctx, c.MD.UserId, peer, id, in.GetSavedReaction())
	})
	return c.messagesFromRows(kept), nil
}

type savedQuery struct {
	Q        string
	MinDate  int32
	MaxDate  int32
	Offset   int32
	Limit    int32
	Saved    *mtproto.PeerUtil
	Reaction []*mtproto.Reaction
	Filter   *mtproto.MessagesFilter
}

func keepSaved(rows []searchbox.Row, q savedQuery, hasReaction func(int32) bool) []searchbox.Row {
	text := strings.ToLower(strings.TrimSpace(q.Q))
	limit := q.Limit
	if limit <= 0 {
		limit = 20
	}
	var out []searchbox.Row
	for _, row := range rows {
		if q.Offset > 0 && row.ID >= q.Offset {
			continue
		}
		if !savedPeerMatch(row.Message, q.Saved) {
			continue
		}
		if !searchDateOK(row.Date, q.MinDate, q.MaxDate) {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(rowText(row)), text) {
			continue
		}
		if !rowAllowsFilter(row, q.Filter) {
			continue
		}
		if len(q.Reaction) > 0 && (hasReaction == nil || !hasReaction(row.ID)) {
			continue
		}
		out = append(out, row)
		if int32(len(out)) >= limit {
			break
		}
	}
	return out
}

func (c *MessagesCore) keepReactionBoxes(peer *mtproto.PeerUtil, list *mtproto.MessageBoxList, want []*mtproto.Reaction) *mtproto.MessageBoxList {
	if list == nil || len(want) == 0 || peer == nil || !peer.IsSelfUser(c.MD.UserId) {
		return list
	}
	kept := make([]*mtproto.MessageBox, 0, len(list.BoxList))
	for _, box := range list.BoxList {
		if box == nil {
			continue
		}
		if chatbox.Shared().HasReaction(c.ctx, c.MD.UserId, peer, box.GetMessageId(), want) {
			kept = append(kept, box)
		}
	}
	list.BoxList = kept
	return list
}

func (c *MessagesCore) messagesFromRows(rows []searchbox.Row) *mtproto.Messages_Messages {
	msgs := messagesOf(rows)
	if msgs == nil {
		msgs = []*mtproto.Message{}
	}
	users, chats := c.peersOf(msgs)
	return mtproto.MakeTLMessagesMessages(&mtproto.Messages_Messages{
		Messages: msgs,
		Chats:    chats,
		Users:    users,
	}).To_Messages_Messages()
}

func rowText(row searchbox.Row) string {
	if row.Text != "" {
		return row.Text
	}
	if row.Message != nil {
		return row.Message.GetMessage()
	}
	return ""
}

func searchDateOK(date, minDate, maxDate int32) bool {
	if minDate > 0 && date < minDate {
		return false
	}
	if maxDate > 0 && date > maxDate {
		return false
	}
	return true
}

func rowAllowsFilter(row searchbox.Row, filter *mtproto.MessagesFilter) bool {
	if filter == nil {
		return true
	}
	switch mtproto.FromMessagesFilter(filter) {
	case mtproto.FilterEmpty:
		return true
	case mtproto.FilterPhotos:
		return mtproto.GetMediaType(row.Message) == mtproto.MEDIA_PHOTOS_ONLY
	case mtproto.FilterVideo:
		return mtproto.GetMediaType(row.Message) == mtproto.MEDIA_VIDEOS_ONLY
	case mtproto.FilterPhotoVideo:
		t := mtproto.GetMediaType(row.Message)
		return t == mtproto.MEDIA_PHOTOS_ONLY || t == mtproto.MEDIA_VIDEOS_ONLY
	case mtproto.FilterDocument:
		return mtproto.GetMediaType(row.Message) == mtproto.MEDIA_FILE
	case mtproto.FilterUrl:
		return mtproto.GetMediaType(row.Message) == mtproto.MEDIA_URL
	case mtproto.FilterGif:
		return mtproto.GetMediaType(row.Message) == mtproto.MEDIA_GIF
	case mtproto.FilterMusic:
		return mtproto.GetMediaType(row.Message) == mtproto.MEDIA_MUSIC
	case mtproto.FilterMyMentions:
		return row.Mentioned
	default:
		return false
	}
}
