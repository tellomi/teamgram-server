// Copyright 2025 Teamgram Authors
//  All rights reserved.
//
// Author: Benqi (wubenqi@gmail.com)
//

package mtproto

import (
	"encoding/json"
	"strings"
)

// Chatlist invite state is stored in DialogFilter.title_STRING so it round-trips
// through dialog_filters.dialog_filter without a new column. Layer 232 encodes
// the visible title from title as TextWithEntities, so this string is not sent
// to current clients. StripChatlistBlob copies the visible title back before
// an older layer encoder reads title_STRING.
const (
	ChatlistInvitesBlobPrefix = "chatlist-invites:"
	ChatlistJoinBlobPrefix    = "chatlist-join:"
)

// IsChatlistBlob reports whether title_STRING holds server-side chatlist state.
func IsChatlistBlob(s string) bool {
	return strings.HasPrefix(s, ChatlistInvitesBlobPrefix) || strings.HasPrefix(s, ChatlistJoinBlobPrefix)
}

// ChatlistInviteCount is the number of exported invites in an owner blob.
func ChatlistInviteCount(blob string) int {
	raw, ok := strings.CutPrefix(blob, ChatlistInvitesBlobPrefix)
	if !ok || raw == "" {
		return 0
	}
	var rows []struct {
		Slug string `json:"slug"`
	}
	if err := json.Unmarshal([]byte(raw), &rows); err != nil {
		return 0
	}
	n := 0
	for _, row := range rows {
		if row.Slug != "" {
			n++
		}
	}
	return n
}

// PreserveChatlistBlob keeps stored invite or join state when a client saves a
// folder without echoing title_STRING. An incoming blob replaces the stored one.
func PreserveChatlistBlob(incoming, existing *DialogFilter) {
	if incoming == nil || existing == nil {
		return
	}
	if IsChatlistBlob(incoming.Title_STRING) {
		return
	}
	if !IsChatlistBlob(existing.Title_STRING) {
		return
	}
	incoming.Title_STRING = existing.Title_STRING
	if ChatlistInviteCount(existing.Title_STRING) > 0 {
		incoming.HasMyInvites = true
	}
}

// StripChatlistBlob replaces a stored blob with the visible folder title.
func StripChatlistBlob(m *DialogFilter) {
	if m == nil || !IsChatlistBlob(m.Title_STRING) {
		return
	}
	title := m.Title
	if m.Title_TEXTWITHENTITIES != nil && m.Title_TEXTWITHENTITIES.GetText() != "" {
		title = m.Title_TEXTWITHENTITIES.GetText()
	}
	m.Title_STRING = title
	if m.Title == "" {
		m.Title = title
	}
}

func UnmarshalDialogFilter(data string) (*DialogFilter, error) {
	dialogFilter := &DialogFilter{}
	if err := json.Unmarshal([]byte(data), dialogFilter); err != nil {
		// c.Logger.Errorf("jsonx.UnmarshalFromString(%v) - error: %v", v, err)
		// continue
		return nil, err
	}

	return dialogFilter.FixData(), nil
}

func (m *DialogFilter) FixData() *DialogFilter {
	if m.Title != "" {
		if m.Title_STRING == "" {
			m.Title_STRING = m.Title
		}
		if m.Title_TEXTWITHENTITIES == nil {
			m.Title_TEXTWITHENTITIES = MakeTLTextWithEntities(&TextWithEntities{
				Text:     m.Title,
				Entities: []*MessageEntity{},
			}).To_TextWithEntities()
		}
	}
	if m.Title_STRING != "" && !IsChatlistBlob(m.Title_STRING) {
		if m.Title == "" {
			m.Title = m.Title_STRING
		}
		if m.Title_TEXTWITHENTITIES == nil {
			m.Title_TEXTWITHENTITIES = MakeTLTextWithEntities(&TextWithEntities{
				Text:     m.Title_STRING,
				Entities: []*MessageEntity{},
			}).To_TextWithEntities()
		}
	}
	if m.Title_TEXTWITHENTITIES != nil {
		if m.Title == "" {
			m.Title = m.Title_TEXTWITHENTITIES.GetText()
		}
		if m.Title_STRING == "" {
			m.Title_STRING = m.Title_TEXTWITHENTITIES.GetText()
		}
	}

	return m
}
