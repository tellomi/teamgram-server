package core

import "testing"

func TestContactNoteMatch(t *testing.T) {
	if !contactNoteMatch("Notebook Pal", "Notebook", "Pal") {
		t.Fatal("full note")
	}
	if !contactNoteMatch("Pal Notebook", "Notebook", "Pal") {
		t.Fatal("reversed words")
	}
	if !contactNoteMatch("名 姓", "名", "姓") {
		t.Fatal("spaced name")
	}
	if !contactNoteMatch("名姓", "名", "姓") {
		t.Fatal("packed name")
	}
	if contactNoteMatch("zzzz", "Notebook", "Pal") {
		t.Fatal("unrelated query matched")
	}
	if contactNoteMatch("   ", "Notebook", "Pal") {
		t.Fatal("blank query matched")
	}
}
