package userinfo

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestEditAndClear(t *testing.T) {
	st := NewMemory()
	ctx := context.Background()
	if _, err := st.Get(ctx, 0); err != mtproto.ErrUserIdInvalid {
		t.Fatalf("invalid get: %v", err)
	}
	empty, err := st.Get(ctx, 9)
	if err != nil || empty.GetPredicateName() != mtproto.Predicate_help_userInfoEmpty {
		t.Fatalf("empty %#v %v", empty, err)
	}
	entity := mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{Offset: 0, Length: 4}).To_MessageEntity()
	got, err := st.Edit(ctx, 3, 9, "note", []*mtproto.MessageEntity{entity})
	if err != nil {
		t.Fatal(err)
	}
	if got.GetPredicateName() != mtproto.Predicate_help_userInfo || got.GetMessage() != "note" || got.GetAuthor() != "3" || len(got.GetEntities()) != 1 {
		t.Fatalf("saved %#v", got)
	}
	again, err := st.Get(ctx, 9)
	if err != nil || again.GetMessage() != "note" {
		t.Fatalf("reload %#v %v", again, err)
	}
	if _, err = st.Edit(ctx, 3, 9, stringsRepeat("a", 4097), nil); err != mtproto.ErrMessageTooLong {
		t.Fatalf("long: %v", err)
	}
	cleared, err := st.Edit(ctx, 3, 9, "  ", nil)
	if err != nil || cleared.GetPredicateName() != mtproto.Predicate_help_userInfoEmpty {
		t.Fatalf("clear %#v %v", cleared, err)
	}
}

func stringsRepeat(s string, n int) string {
	out := make([]byte, 0, len(s)*n)
	for i := 0; i < n; i++ {
		out = append(out, s...)
	}
	return string(out)
}
