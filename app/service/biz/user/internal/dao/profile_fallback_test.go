package dao

import (
	"strings"
	"testing"
)

func TestReplacingFallbackDeletesTheOldPublicPhoto(t *testing.T) {
	rows := []profilePhoto{
		{id: 1, fallback: false}, // main avatar, fallback = 0
		{id: 2, fallback: true},  // current public photo
		{id: 3, fallback: false}, // ordinary album photo
	}
	got := hidePreviousFallbacks(rows, 9)
	if got[0].deleted || got[0].fallback {
		t.Fatalf("main avatar changed: %+v", got[0])
	}
	if !got[1].deleted || got[1].fallback {
		t.Fatalf("old public photo stayed in the album: %+v", got[1])
	}
	if got[2].deleted {
		t.Fatalf("album photo was deleted: %+v", got[2])
	}

	same := hidePreviousFallbacks([]profilePhoto{{id: 9, fallback: true}}, 9)
	if same[0].deleted {
		t.Fatal("setting the same public photo deleted it")
	}
}

func TestHidePreviousFallbackSQLDoesNotTouchTheMainAvatar(t *testing.T) {
	q := strings.ToLower(hidePreviousFallbackSQL)
	for _, part := range []string{"deleted = 1", "fallback = 1", "deleted = 0", "photo_id <> ?", "user_id = ?"} {
		if !strings.Contains(q, part) {
			t.Fatalf("sql missing %q:\n%s", part, hidePreviousFallbackSQL)
		}
	}
	if strings.Contains(q, "users") || strings.Contains(q, "photo_id =") && strings.Contains(q, "update users") {
		t.Fatal("fallback replace writes the users table")
	}
}

type profilePhoto struct {
	id       int64
	fallback bool
	deleted  bool
}

// hidePreviousFallbacks is the row effect of hidePreviousFallbackSQL.
func hidePreviousFallbacks(rows []profilePhoto, nextID int64) []profilePhoto {
	out := append([]profilePhoto(nil), rows...)
	for i := range out {
		if out[i].fallback && !out[i].deleted && out[i].id != nextID {
			out[i].deleted = true
			out[i].fallback = false
		}
	}
	return out
}
