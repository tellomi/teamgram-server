package push

import (
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestReactionPushUsesReactKey(t *testing.T) {
	up := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdateMessageReactions(&mtproto.Update{
		MsgId_INT32: 7,
		Reactions_MESSAGEREACTIONS: mtproto.MakeTLMessageReactions(&mtproto.MessageReactions{
			Results: []*mtproto.ReactionCount{
				mtproto.MakeTLReactionCount(&mtproto.ReactionCount{
					Reaction_REACTION: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
					Count:             1,
				}).To_ReactionCount(),
			},
			RecentReactions: []*mtproto.MessagePeerReaction{
				mtproto.MakeTLMessagePeerReaction(&mtproto.MessagePeerReaction{
					Unread:            true,
					PeerId:            mtproto.MakePeerUser(4),
					Reaction_REACTION: mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
				}).To_MessagePeerReaction(),
			},
		}).To_MessageReactions(),
	}).To_Update())
	if !ShouldDeliver(9, up) {
		t.Fatal("a reaction on someone else's message did not qualify for push")
	}
	got := Messages(9, up)
	if len(got) != 1 || got[0].LocKey != "REACT_NOTEXT" || got[0].SenderID != 4 || got[0].MsgID != 7 {
		t.Fatalf("reaction push %+v", got)
	}
	if Messages(4, up) != nil {
		t.Fatal("the reactor should not get a REACT_ push for their own reaction")
	}
	alert, err := BuildAlert(got[0], Decision{Send: true, Preview: true, Sound: "default"}, nil, "en")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(alert.Outer), "REACT_NOTEXT") {
		t.Fatalf("loc-key %s", alert.Outer)
	}
}
