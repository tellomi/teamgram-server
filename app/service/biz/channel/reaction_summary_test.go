package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestReactionSummaryIsEmojiAndCount(t *testing.T) {
	summary := reactionSummary([]ReactRec{
		{MsgID: 4, UserID: 9, Emoticon: "👍", Date: 1},
		{MsgID: 4, UserID: 3, Emoticon: "👍", Date: 2},
		{MsgID: 8, UserID: 9, Emoticon: "🔥"},
	}, 4, 9, true, 0)
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
	if !summary.GetCanSeeList() || len(summary.GetRecentReactions()) != 2 {
		t.Fatalf("recent %#v", summary)
	}
	if summary.GetRecentReactions()[0].GetPeerId().GetUserId() != 3 {
		t.Fatalf("newest reactor %#v", summary.GetRecentReactions()[0])
	}
	hidden := reactionSummary([]ReactRec{
		{MsgID: 4, UserID: 9, Emoticon: "👍"},
	}, 4, 9, false, 0)
	if hidden.GetCanSeeList() || len(hidden.GetRecentReactions()) != 0 {
		t.Fatalf("broadcast summary %#v", hidden)
	}
}
