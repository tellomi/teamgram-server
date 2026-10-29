package mtproto

import (
	"bytes"
	"testing"
	"unicode/utf16"
)

// TestGetDifferenceSurvivesDirtyEntities is the regression for a single bad
// history row freezing official clients on "Updating...". The text has a
// surrogate-pair emoji and a trailing newline. Entities are a zero-length
// hashtag, the same URL twice, and one URL whose offset is past the UTF-16
// length. A second message panics while encoding (nil peer). The difference
// must still serialize, and its pts must be written.
func TestGetDifferenceSurvivesDirtyEntities(t *testing.T) {
	const (
		url = "https://tell.cc/a"
		pts = int32(42)
	)
	text := "😀 " + url + "\n"
	if UTF16Len("😀") != 2 {
		t.Fatalf("emoji UTF-16 length = %d, want 2 (surrogate pair)", UTF16Len("😀"))
	}
	if utf16.Encode([]rune("😀"))[0] < 0xD800 {
		t.Fatal("emoji did not encode as a surrogate pair")
	}
	urlAt := int32(UTF16Len("😀 "))
	urlLen := int32(UTF16Len(url))
	if int(urlAt)+int(urlLen) > UTF16Len(text) {
		t.Fatalf("url %d+%d past text %d", urlAt, urlLen, UTF16Len(text))
	}

	good := MakeTLMessage(&Message{
		Id:      7,
		PeerId:  MakeTLPeerUser(&Peer{UserId: 1}).To_Peer(),
		Date:    1,
		Message: text,
		Entities: []*MessageEntity{
			MakeTLMessageEntityHashtag(&MessageEntity{Offset: 0, Length: 0}).To_MessageEntity(),
			MakeTLMessageEntityUrl(&MessageEntity{Offset: urlAt, Length: urlLen}).To_MessageEntity(),
			MakeTLMessageEntityUrl(&MessageEntity{Offset: urlAt, Length: urlLen}).To_MessageEntity(),
			MakeTLMessageEntityBold(&MessageEntity{Offset: int32(UTF16Len(text)) + 1, Length: 1}).To_MessageEntity(),
		},
	}).To_Message()
	// Nil peer panics inside the generated encoder. One row must not abort the difference.
	bad := MakeTLMessage(&Message{
		Id:      8,
		Message: "panic",
		Entities: []*MessageEntity{
			nil,
		},
	}).To_Message()

	diff := MakeTLUpdatesDifference(&Updates_Difference{
		NewMessages:          []*Message{good, bad},
		NewEncryptedMessages: []*EncryptedMessage{},
		OtherUpdates:         []*Update{},
		Chats:                []*Chat{},
		Users:                []*User{},
		State: MakeTLUpdatesState(&Updates_State{
			Pts:  pts,
			Date: 1,
			Seq:  3,
		}).To_Updates_State(),
	}).To_Updates_Difference()
	SanitizeOutgoingObject(diff)

	urls := 0
	for _, entity := range good.GetEntities() {
		if entity.GetLength() <= 0 {
			t.Fatalf("zero-length entity survived read filter: %#v", entity)
		}
		if int64(entity.Offset)+int64(entity.Length) > int64(UTF16Len(good.GetMessage())) {
			t.Fatalf("entity out of UTF-16 range: off=%d len=%d text=%d", entity.Offset, entity.Length, UTF16Len(good.GetMessage()))
		}
		if entity.GetPredicateName() == Predicate_messageEntityUrl {
			urls++
			if entity.Offset != urlAt || entity.Length != urlLen {
				t.Fatalf("url utf-16 span = %d+%d, want %d+%d", entity.Offset, entity.Length, urlAt, urlLen)
			}
		}
		if entity.GetPredicateName() == Predicate_messageEntityHashtag {
			t.Fatal("zero-length hashtag was kept")
		}
	}
	if urls != 1 {
		t.Fatalf("url entities = %d, want 1", urls)
	}

	const layer int32 = 232
	buf := NewEncodeBuf(512)
	if err := diff.Encode(buf, layer); err != nil {
		t.Fatalf("getDifference encode: %v", err)
	}
	if diff.GetState().GetPts() != pts {
		t.Fatalf("pts = %d, want %d", diff.GetState().GetPts(), pts)
	}
	stateBuf := NewEncodeBuf(64)
	if err := diff.GetState().Encode(stateBuf, layer); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(buf.GetBuf(), stateBuf.GetBuf()) {
		t.Fatal("difference state was not serialized; a bad message aborted getDifference")
	}
}
