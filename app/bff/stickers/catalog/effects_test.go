package catalog

import "testing"

func TestAvailableEffectsAndEmojiURL(t *testing.T) {
	s := Must()
	page := s.AvailableEffects(0)
	if page.GetPredicateName() != "messages_availableEffects" && len(page.GetEffects()) == 0 {
		t.Fatalf("effects %+v", page)
	}
	if len(page.GetEffects()) == 0 || len(page.GetDocuments()) == 0 {
		t.Fatalf("empty effects %+v", page)
	}
	if page.GetEffects()[0].GetEmoticon() == "" || page.GetEffects()[0].GetEffectStickerId() == 0 {
		t.Fatalf("effect %+v", page.GetEffects()[0])
	}
	again := s.AvailableEffects(page.GetHash())
	if again.GetPredicateName() != "messages_availableEffectsNotModified" {
		t.Fatalf("hash %+v", again)
	}
	tags := s.DefaultTagReactions(0)
	if len(tags.GetReactions()) < 8 {
		t.Fatalf("tags %+v", tags)
	}
	url, err := s.EmojiURL("en")
	if err != nil || url.GetUrl() == "" {
		t.Fatalf("url %+v %v", url, err)
	}
	if _, err := s.EmojiURL(""); err == nil {
		t.Fatal("empty lang accepted")
	}
	if _, err := s.EmojiURL("english"); err == nil {
		t.Fatal("english accepted")
	}
}
