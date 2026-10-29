package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestReactionSummaryIsEmojiAndCount(t *testing.T) {
	summary := reactionSummary([]ReactRec{
		{MsgID: 4, UserID: 9, Emoticon: "👍"},
		{MsgID: 4, UserID: 3, Emoticon: "👍"},
		{MsgID: 8, UserID: 9, Emoticon: "🔥"},
	}, 4, 9)
	if summary.GetPredicateName() != mtproto.Predicate_messageReactions {
		t.Fatalf("predicate %s", summary.GetPredicateName())
	}
	results := summary.GetResults()
	if len(results) != 1 {
		t.Fatalf("results %#v", results)
	}
	rc := results[0]
	if rc.GetCount() != 2 {
		t.Fatalf("count %d", rc.GetCount())
	}
	reaction := rc.GetReaction_REACTION()
	if reaction.GetPredicateName() != mtproto.Predicate_reactionEmoji || reaction.GetEmoticon() != "👍" {
		t.Fatalf("reaction %#v", reaction)
	}
	if rc.GetReaction_STRING() != "👍" {
		t.Fatalf("legacy reaction string %q", rc.GetReaction_STRING())
	}
	if rc.GetChosenOrder() == nil || rc.GetChosenOrder().GetValue() != 0 {
		t.Fatal("chooser should be marked without a star count")
	}
}
