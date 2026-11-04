package catalog

import (
	"bytes"
	"image/jpeg"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestBundledGifsArePlayableDocuments(t *testing.T) {
	s := newService(builtinPacks())
	saved := s.SavedGifs(9, 0)
	if saved.GetPredicateName() != mtproto.Predicate_messages_savedGifs || len(saved.GetGifs()) < 4 {
		t.Fatalf("saved gifs %d %s", len(saved.GetGifs()), saved.GetPredicateName())
	}
	for _, doc := range saved.GetGifs() {
		if doc.GetMimeType() != "video/mp4" || doc.GetDcId() == 0 || len(doc.GetThumbs()) == 0 || len(doc.GetVideoThumbs()) == 0 {
			t.Fatalf("gif document missing thumb/dc %#v", doc)
		}
		var animated, mp4Name bool
		for _, attr := range doc.GetAttributes() {
			switch attr.GetPredicateName() {
			case mtproto.Predicate_documentAttributeAnimated:
				animated = true
			case mtproto.Predicate_documentAttributeFilename:
				mp4Name = len(attr.GetFileName()) > 4 && attr.GetFileName()[len(attr.GetFileName())-4:] == ".mp4"
			}
		}
		if !animated || !mp4Name {
			t.Fatalf("gif attributes %#v", doc.GetAttributes())
		}
		body, mime, ok := s.DocumentFile(doc.GetId())
		if !ok || mime != "video/mp4" || len(body) < 12 || string(body[4:8]) != "ftyp" {
			t.Fatalf("download id %d mime %s len %d", doc.GetId(), mime, len(body))
		}
		jpg, thumbMIME, ok := GifThumb(doc.GetId(), "m")
		if !ok || thumbMIME != "image/jpeg" {
			t.Fatal("missing jpeg thumb")
		}
		if _, err := jpeg.Decode(bytes.NewReader(jpg)); err != nil {
			t.Fatal(err)
		}
		vid, vidMIME, ok := GifThumb(doc.GetId(), "v")
		if !ok || vidMIME != "video/mp4" || !bytes.Equal(vid, body) {
			t.Fatal("video thumb is not the mp4")
		}
	}
	again := s.SavedGifs(9, saved.GetHash())
	if again.GetPredicateName() != mtproto.Predicate_messages_savedGifsNotModified {
		t.Fatalf("hash %s", again.GetPredicateName())
	}
	red := bundledGifDocuments("red")
	if len(red) != 1 {
		t.Fatalf("search red %d", len(red))
	}
	bot := GifBotUser()
	if !bot.GetBot() || bot.GetId() != mtproto.BotGifId || bot.GetUsername().GetValue() != "gif" {
		t.Fatalf("gif bot %#v", bot)
	}
	hits := GifInlineResults("")
	if len(hits) != len(saved.GetGifs()) || hits[0].GetDocument() == nil || hits[0].GetType() != "gif" {
		t.Fatalf("inline %#v", hits)
	}
}
