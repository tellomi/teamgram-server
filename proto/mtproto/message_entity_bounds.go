// Copyright 2026 Teamgram Authors
//  All rights reserved.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package mtproto

import (
	"sort"
)

// UTF16Len returns the length of s in UTF-16 code units, which is the unit
// Telegram uses for message entity offset and length.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16Units(r)
	}
	return n
}

func utf16Units(r rune) int {
	if r >= 0x10000 {
		return 2
	}
	return 1
}

// entityInRange reports whether e covers a non-empty span inside a text of
// n UTF-16 code units. length <= 0 is rejected: official iOS clients abort
// getDifference when they see a zero-length entity and stay on "Updating...".
func entityInRange(e *MessageEntity, n int) bool {
	if e == nil || e.Length <= 0 || e.Offset < 0 {
		return false
	}
	end := int64(e.Offset) + int64(e.Length)
	return end <= int64(n)
}

type entityIdentity struct {
	typ                string
	offset, length     int32
	url, language      string
	oldText            string
	userID, documentID int64
	collapsed          bool
	relative           bool
	shortTime          bool
	longTime           bool
	shortDate          bool
	longDate           bool
	dayOfWeek          bool
	date               int32
}

func entityTypeName(e *MessageEntity) string {
	if name := e.GetPredicateName(); name != "" {
		return name
	}
	if name, ok := clazzIdNameRegisters2[int32(e.GetConstructor())]; ok {
		return name
	}
	if e.GetConstructor() != 0 {
		return e.GetConstructor().String()
	}
	return ""
}

func entityKey(e *MessageEntity) entityIdentity {
	userID := e.GetUserId_INT64()
	if userID == 0 && e.GetUserId_INPUTUSER() != nil {
		userID = e.GetUserId_INPUTUSER().GetUserId()
	}
	return entityIdentity{
		typ:        entityTypeName(e),
		offset:     e.GetOffset(),
		length:     e.GetLength(),
		url:        e.GetUrl(),
		language:   e.GetLanguage(),
		oldText:    e.GetOldText(),
		userID:     userID,
		documentID: e.GetDocumentId(),
		collapsed:  e.GetCollapsed(),
		relative:   e.GetRelative(),
		shortTime:  e.GetShortTime(),
		longTime:   e.GetLongTime(),
		shortDate:  e.GetShortDate(),
		longDate:   e.GetLongDate(),
		dayOfWeek:  e.GetDayOfWeek(),
		date:       e.GetDate(),
	}
}

// SanitizeMessageEntities drops entities official clients cannot apply:
// length <= 0, a negative offset, or offset+length past the UTF-16 length of
// text. Identical entities are collapsed. The result is ordered by offset.
// A nil slice is returned when nothing remains so the entities flag stays unset.
func SanitizeMessageEntities(text string, entities []*MessageEntity) []*MessageEntity {
	if len(entities) == 0 {
		return nil
	}
	n := UTF16Len(text)
	kept := make([]*MessageEntity, 0, len(entities))
	seen := make(map[entityIdentity]struct{}, len(entities))
	for _, entity := range entities {
		if !entityInRange(entity, n) {
			continue
		}
		key := entityKey(entity)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		kept = append(kept, entity)
	}
	if len(kept) == 0 {
		return nil
	}
	sort.SliceStable(kept, func(i, j int) bool {
		if kept[i].Offset != kept[j].Offset {
			return kept[i].Offset < kept[j].Offset
		}
		if kept[i].Length != kept[j].Length {
			return kept[i].Length < kept[j].Length
		}
		return entityTypeName(kept[i]) < entityTypeName(kept[j])
	})
	return kept
}

// EntityOverlaps reports whether the UTF-16 spans of a and b intersect.
// Empty spans do not overlap.
func EntityOverlaps(a, b *MessageEntity) bool {
	if a == nil || b == nil || a.Length <= 0 || b.Length <= 0 {
		return false
	}
	a0 := int64(a.Offset)
	b0 := int64(b.Offset)
	return a0 < b0+int64(b.Length) && b0 < a0+int64(a.Length)
}

// SanitizeEntities drops invalid and duplicate entities on the message text
// and on a quote attached to the message.
func (m *Message) SanitizeEntities() {
	if m == nil {
		return
	}
	m.Entities = SanitizeMessageEntities(m.GetMessage(), m.Entities)
	if hdr := m.GetReplyTo(); hdr != nil && (hdr.QuoteEntities != nil || hdr.GetQuoteText() != nil) {
		quote := ""
		if hdr.GetQuoteText() != nil {
			quote = hdr.GetQuoteText().GetValue()
		}
		hdr.QuoteEntities = SanitizeMessageEntities(quote, hdr.QuoteEntities)
	}
}

// SanitizeEmbeddedEntities cleans entities carried by an update, either on
// the embedded message or on the update itself (service notifications and
// short messages). It does not adjust TTL; that stays in FixData.
func (m *Update) SanitizeEmbeddedEntities() {
	if m == nil {
		return
	}
	if msg := m.GetMessage_MESSAGE(); msg != nil {
		msg.SanitizeEntities()
	}
	if m.Entities != nil {
		m.Entities = SanitizeMessageEntities(m.GetMessage_STRING(), m.Entities)
	}
}

// SanitizeEntities drops invalid caption entities. Story captions use the
// same UTF-16 entity rules as messages.
func (m *StoryItem) SanitizeEntities() {
	if m == nil {
		return
	}
	caption := ""
	if m.GetCaption() != nil {
		caption = m.GetCaption().GetValue()
	}
	m.Entities = SanitizeMessageEntities(caption, m.Entities)
}

// SanitizeEntities drops invalid draft entities before they are stored or
// returned to a client.
func (m *DraftMessage) SanitizeEntities() {
	if m == nil {
		return
	}
	m.Entities = SanitizeMessageEntities(m.GetMessage(), m.GetEntities())
}

// SanitizeOutgoing applies the same UTF-16 bounds to a short update
// (updateShortMessage carries entities on Updates, not on Message).
func (m *Updates) SanitizeOutgoing() {
	if m == nil {
		return
	}
	if m.Entities != nil || m.Message != "" {
		m.Entities = SanitizeMessageEntities(m.Message, m.Entities)
	}
	if m.Update != nil {
		m.Update.SanitizeEmbeddedEntities()
	}
	for _, update := range m.Updates {
		if update != nil {
			update.SanitizeEmbeddedEntities()
		}
	}
}

// SanitizeOutgoingObject filters entities on the objects official clients
// receive from getDifference, getHistory, and push. Stored rows are also
// cleaned when they are loaded; this is the last step before those objects
// are serialized onto the connection.
func SanitizeOutgoingObject(obj TLObject) {
	switch o := obj.(type) {
	case *Updates:
		o.SanitizeOutgoing()
	case *Update:
		o.SanitizeEmbeddedEntities()
	case *Message:
		o.SanitizeEntities()
	case *Updates_Difference:
		if o == nil {
			return
		}
		for _, msg := range o.NewMessages {
			if msg != nil {
				msg.SanitizeEntities()
			}
		}
		for _, update := range o.OtherUpdates {
			if update != nil {
				update.SanitizeEmbeddedEntities()
			}
		}
	case *Updates_ChannelDifference:
		if o == nil {
			return
		}
		for _, msg := range o.NewMessages {
			if msg != nil {
				msg.SanitizeEntities()
			}
		}
		for _, msg := range o.Messages {
			if msg != nil {
				msg.SanitizeEntities()
			}
		}
		for _, update := range o.OtherUpdates {
			if update != nil {
				update.SanitizeEmbeddedEntities()
			}
		}
	case *Messages_Messages:
		if o == nil {
			return
		}
		for _, msg := range o.Messages {
			if msg != nil {
				msg.SanitizeEntities()
			}
		}
	case *TLRpcResult:
		if o != nil {
			SanitizeOutgoingObject(o.Result)
		}
	}
}
