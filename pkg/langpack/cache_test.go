package langpack

import (
	"testing"
	"time"
)

func TestLangPackDifferenceIsCached(t *testing.T) {
	c := DefaultCatalog()
	if c == nil {
		t.Fatal(DefaultError())
	}
	start := time.Now()
	cold, err := c.Difference("android", "zh-hans", 0)
	coldElapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	warm, err := c.Difference("android", "zh-hans", 0)
	warmElapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if cold != warm {
		t.Fatal("getLangPack rebuilt the payload")
	}
	if cold.GetVersion() == 0 || len(cold.GetStrings()) == 0 {
		t.Fatalf("empty pack version %d strings %d", cold.GetVersion(), len(cold.GetStrings()))
	}
	// A different from_version is a different payload.
	partial, err := c.Difference("android", "zh-hans", cold.GetVersion())
	if err != nil {
		t.Fatal(err)
	}
	if partial == cold {
		t.Fatal("cached the full pack for a client that is already current")
	}
	t.Logf("langpack zh-hans cold %s warm %s strings %d", coldElapsed, warmElapsed, len(cold.GetStrings()))
	// Clients still below the Android XML cutover all receive the full pack.
	// That pack is built once, not once per from_version.
	below, err := c.Difference("android", "zh-hans", 1)
	if err != nil {
		t.Fatal(err)
	}
	again, err := c.Difference("android", "zh-hans", 2)
	if err != nil {
		t.Fatal(err)
	}
	if below != again || below.GetFromVersion() != 0 {
		t.Fatal("android xml reset rebuilt the full pack per from_version")
	}
}

func TestLangPackCacheFollowsVersion(t *testing.T) {
	lang := &Language{
		Code: "en", Name: "English", Version: 1,
		Strings: []Entry{{Key: "A", Value: "one", Version: 1}},
	}
	cat := NewCatalog(map[string]map[string]*Language{"android": {"en": lang}})
	first, err := cat.Difference("android", "en", 0)
	if err != nil || first.GetVersion() != 1 || len(first.GetStrings()) != 1 {
		t.Fatalf("%+v %v", first, err)
	}
	loaded := cat.platforms["android"]["en"]
	loaded.meta.Version = 2
	loaded.meta.Strings = append(loaded.meta.Strings, Entry{Key: "B", Value: "two", Version: 2})
	second, err := cat.Difference("android", "en", 0)
	if err != nil {
		t.Fatal(err)
	}
	if second == first || second.GetVersion() != 2 || len(second.GetStrings()) != 2 {
		t.Fatalf("old pack served again: version %d strings %d same %v", second.GetVersion(), len(second.GetStrings()), second == first)
	}
	warm, err := cat.Difference("android", "en", 0)
	if err != nil || warm != second {
		t.Fatal("new version was not cached")
	}
	one, err := cat.Strings("android", "en", []string{"A", "missing"})
	if err != nil || len(one) != 2 || one[0].GetValue() != "one" || one[1].GetPredicateName() != "langPackStringDeleted" {
		t.Fatalf("%+v %v", one, err)
	}
	two, err := cat.Strings("android", "en", []string{"A"})
	if err != nil || len(two) != 1 || two[0] != one[0] {
		t.Fatal("getStrings rebuilt the cached string")
	}
}
