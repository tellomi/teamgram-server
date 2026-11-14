package core

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// replyHeaderChecked copies inputReplyToMessage and checks quote_text against
// the message being replied to. A quote that is not in that message is
// QUOTE_TEXT_INVALID. todo_item_id is copied by replyHeaderFromInput.
// https://core.telegram.org/constructor/inputReplyToMessage
func (c *MessagesCore) replyHeaderChecked(dest *mtproto.PeerUtil, in *mtproto.InputReplyTo) (*mtproto.MessageReplyHeader, error) {
	header := replyHeaderFromInput(c.MD.UserId, dest, in)
	if header == nil || in.GetQuoteText() == nil {
		return header, nil
	}
	msg, err := c.loadReplyMessage(in.GetReplyToMsgId())
	if err != nil || msg == nil {
		return nil, mtproto.ErrQuoteTextInvalid
	}
	if !quoteTextValid(quoteSource(msg), in.GetQuoteText().GetValue(), in.GetQuoteOffset()) {
		return nil, mtproto.ErrQuoteTextInvalid
	}
	return header, nil
}

func (c *MessagesCore) loadReplyMessage(id int32) (*mtproto.Message, error) {
	if id <= 0 || c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MessageClient == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	box, err := c.svcCtx.Dao.MessageClient.MessageGetUserMessage(c.ctx, &message.TLMessageGetUserMessage{
		UserId: c.MD.UserId,
		Id:     id,
	})
	if err != nil {
		return nil, err
	}
	if box == nil || box.GetMessage() == nil {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return box.GetMessage(), nil
}

func quoteSource(m *mtproto.Message) string {
	if m == nil {
		return ""
	}
	return m.GetMessage()
}

// quoteTextValid reports whether quote is the UTF-16 slice of source that
// starts at offset. A missing offset is 0. Official clients measure quote
// offsets in UTF-16 code units, the same unit as message entities.
func quoteTextValid(source, quote string, offset *wrapperspb.Int32Value) bool {
	if quote == "" || mtproto.UTF16Len(quote) > 1024 {
		return false
	}
	start := 0
	if offset != nil {
		start = int(offset.GetValue())
	}
	if start < 0 {
		return false
	}
	got, ok := utf16Slice(source, start, mtproto.UTF16Len(quote))
	return ok && got == quote
}

func utf16Slice(s string, offset, length int) (string, bool) {
	if offset < 0 || length < 0 {
		return "", false
	}
	var b strings.Builder
	pos := 0
	started := false
	for _, r := range s {
		units := 1
		if r > 0xFFFF {
			units = 2
		}
		if !started {
			if pos == offset {
				started = true
			} else if pos > offset {
				return "", false
			}
		}
		if started {
			if pos >= offset+length {
				break
			}
			if pos+units > offset+length {
				return "", false
			}
			b.WriteRune(r)
		}
		pos += units
	}
	if !started || pos < offset+length {
		return "", false
	}
	return b.String(), true
}
