package book

import (
	"context"
	"encoding/json"
	"strconv"
	"unicode/utf16"

	"github.com/teamgram/proto/mtproto"
)

// Contact notes are private to the owner and shown on userFull.note.
// https://core.telegram.org/method/contacts.updateContactNote
//
// The length cap matches the client contact_note_length_limit default of 128
// UTF-16 code units. Entity offsets use the same UTF-16 positions as the rest
// of the API (https://core.telegram.org/api/entities#entity-length).

const (
	noteKeyPrefix = "contacts_note_v1:"
	NoteLimit     = 128
	maxEntities   = 100
)

type noteRec struct {
	Text     string                   `json:"text"`
	Entities []*mtproto.MessageEntity `json:"entities,omitempty"`
}

// PutNote stores a note, or deletes it when the text is empty.
func (b *Book) PutNote(ctx context.Context, owner, peer int64, note *mtproto.TextWithEntities) error {
	if note == nil || note.GetText() == "" {
		_, err := b.kv.DelCtx(ctx, noteKey(owner, peer))
		return err
	}
	if err := ValidateNote(note); err != nil {
		return err
	}
	raw, err := json.Marshal(noteRec{Text: note.GetText(), Entities: note.GetEntities()})
	if err != nil {
		return err
	}
	return b.kv.SetCtx(ctx, noteKey(owner, peer), string(raw))
}

// Note returns the owner's private note about peer. Nil means there is none.
func (b *Book) Note(ctx context.Context, owner, peer int64) *mtproto.TextWithEntities {
	if b == nil || owner == 0 || peer == 0 || owner == peer {
		return nil
	}
	raw, err := b.kv.GetCtx(ctx, noteKey(owner, peer))
	if err != nil || raw == "" {
		return nil
	}
	var rec noteRec
	if json.Unmarshal([]byte(raw), &rec) != nil || rec.Text == "" {
		return nil
	}
	return mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
		Text:     rec.Text,
		Entities: rec.Entities,
	}).To_TextWithEntities()
}

// ValidateNote checks length and entity bounds.
func ValidateNote(note *mtproto.TextWithEntities) error {
	if note == nil || note.GetText() == "" {
		return nil
	}
	n := utf16Len(note.GetText())
	if n > NoteLimit {
		return mtproto.ErrMessageTooLong
	}
	ents := note.GetEntities()
	if len(ents) > maxEntities {
		return mtproto.ErrEntitiesTooLong
	}
	for _, e := range ents {
		if e == nil || e.GetLength() <= 0 || e.GetOffset() < 0 || e.GetOffset()+e.GetLength() > n {
			return mtproto.ErrEntityBoundsInvalid
		}
	}
	return nil
}

func noteKey(owner, peer int64) string {
	return noteKeyPrefix + strconv.FormatInt(owner, 10) + ":" + strconv.FormatInt(peer, 10)
}

func utf16Len(s string) int32 {
	return int32(len(utf16.Encode([]rune(s))))
}
