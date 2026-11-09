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
}
