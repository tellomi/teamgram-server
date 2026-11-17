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

func TestCallerSavedBucketsMyResults(t *testing.T) {
	book := map[int64]struct{}{42: {}}
	if !callerSaved(book, 42) {
		t.Fatal("one-way contact must be my_results")
	}
	if callerSaved(book, 7) {
		t.Fatal("stranger must stay in results")
	}
	if callerSaved(nil, 42) || callerSaved(book, 0) {
		t.Fatal("empty book or zero id")
	}
}
