package chatbox

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestQuizCorrectAnswerAndSolution(t *testing.T) {
	b := newBook(nil)
	ctx := context.Background()
	media, err := b.CreatePoll(ctx, 1, &mtproto.Poll{
		Quiz:     true,
		Question: "Two plus two",
		Answers: []*mtproto.PollAnswer{
			mtproto.MakePollAnswerWithText("3", []byte("0")),
			mtproto.MakePollAnswerWithText("4", []byte("1")),
		},
	}, [][]byte{[]byte("1")}, "Because arithmetic", []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityBold(&mtproto.MessageEntity{Offset: 0, Length: 7}).To_MessageEntity(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if media.GetPredicateName() != mtproto.Predicate_messageMediaPoll {
		t.Fatalf("media %s", media.GetPredicateName())
	}
	pollID := media.GetPoll().GetId()
	b.BindPoll(ctx, mtproto.PEER_CHAT, 8, 3, pollID)

	wrong, err := b.Vote(ctx, 4, mtproto.PEER_CHAT, 8, 3, 0, [][]byte{[]byte("0")})
	if err != nil {
		t.Fatal(err)
	}
	res := wrong.Updates[0].GetResults()
	if res.GetSolution().GetValue() != "Because arithmetic" || len(res.GetSolutionEntities()) != 1 {
		t.Fatalf("solution %+v", res)
	}
	var sawCorrect, sawChosen bool
	for _, row := range res.GetResults() {
		if string(row.GetOption()) == "1" && row.GetCorrect() {
			sawCorrect = true
		}
		if string(row.GetOption()) == "0" && row.GetChosen() && !row.GetCorrect() {
			sawChosen = true
		}
	}
	if !sawCorrect || !sawChosen {
		t.Fatalf("rows %+v", res.GetResults())
	}
	if _, err := b.Vote(ctx, 4, mtproto.PEER_CHAT, 8, 3, pollID, [][]byte{[]byte("1")}); err != mtproto.ErrRevoteNotAllowed {
		t.Fatalf("quiz revote: %v", err)
	}

	// The author sees the solution before answering. A stranger does not.
	author, err := b.PollResultsUpdate(ctx, 1, mtproto.PEER_CHAT, 8, 3, pollID, 0)
	if err != nil || author.Updates[0].GetResults().GetSolution().GetValue() != "Because arithmetic" {
		t.Fatalf("author %+v %v", author, err)
	}
	stranger, err := b.PollUpdateFor(ctx, 9, mtproto.PEER_CHAT, 8, 3, pollID)
	if err != nil {
		t.Fatal(err)
	}
	if stranger.Updates[0].GetResults().GetSolution() != nil {
		t.Fatal("solution leaked before the vote")
	}
	_ = wrapperspb.Int32(0)
}

func TestPrivatePollUsesDialogMessageID(t *testing.T) {
	b := newBook(nil)
	b.ids = fakeIDs{box: map[int64]map[int32]int64{
		1: {11: 900},
		2: {44: 900},
	}}
	ctx := context.Background()
	media, err := b.CreatePoll(ctx, 1, &mtproto.Poll{
		Question: "Go",
		Answers: []*mtproto.PollAnswer{
			mtproto.MakePollAnswerWithText("yes", []byte("0")),
			mtproto.MakePollAnswerWithText("no", []byte("1")),
		},
	}, nil, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// Sender 1, peer 2, sender's box id 11.
	b.BindPoll(ctx, mtproto.PEER_USER, 2, 11, media.GetPoll().GetId())
	ups, err := b.Vote(ctx, 2, mtproto.PEER_USER, 1, 44, 0, [][]byte{[]byte("0")})
	if err != nil {
		t.Fatal(err)
	}
	if ups.Updates[0].GetMsgId_INT32() != 44 {
		t.Fatalf("voter update id %d", ups.Updates[0].GetMsgId_INT32())
	}
	other, err := b.PollUpdateFor(ctx, 1, mtproto.PEER_USER, 2, 11, 0)
	if err != nil {
		t.Fatal(err)
	}
	if other.Updates[0].GetMsgId_INT32() != 11 {
		t.Fatalf("author update id %d", other.Updates[0].GetMsgId_INT32())
	}
	if ups.Updates[0].GetPeer_PEER().GetUserId() != 1 {
		t.Fatalf("voter peer %d", ups.Updates[0].GetPeer_PEER().GetUserId())
	}
	if other.Updates[0].GetPeer_PEER().GetUserId() != 2 {
		t.Fatalf("author peer %d", other.Updates[0].GetPeer_PEER().GetUserId())
	}
	if other.Updates[0].GetResults().GetTotalVoters().GetValue() != 1 {
		t.Fatalf("author totals %+v", other.Updates[0].GetResults())
	}
}

func TestPollsDisabledLeavesMediaUnsupported(t *testing.T) {
	t.Setenv("TELLOMI_POLLS", "")
	if PollsEnabled() {
		t.Fatal("polls should be off")
	}
	media, err := PollInputMedia(context.Background(), 1, mtproto.MakeTLInputMediaPoll(&mtproto.InputMedia{
		Poll: &mtproto.Poll{
			Question: "Q",
			Answers: []*mtproto.PollAnswer{
				mtproto.MakePollAnswerWithText("a", []byte("0")),
				mtproto.MakePollAnswerWithText("b", []byte("1")),
			},
		},
	}).To_InputMedia())
	if err != nil || media.GetPredicateName() != mtproto.Predicate_messageMediaUnsupported {
		t.Fatalf("off %+v %v", media, err)
	}
	t.Setenv("TELLOMI_POLLS", "1")
	media, err = PollInputMedia(context.Background(), 1, mtproto.MakeTLInputMediaPoll(&mtproto.InputMedia{
		Poll: &mtproto.Poll{
			Quiz:     true,
			Question: "Q",
			Answers: []*mtproto.PollAnswer{
				mtproto.MakePollAnswerWithText("a", []byte("0")),
				mtproto.MakePollAnswerWithText("b", []byte("1")),
			},
		},
		CorrectAnswers_FLAGVECTORBYTES: [][]byte{[]byte("1")},
		Solution:                       wrapperspb.String("s"),
	}).To_InputMedia())
	if err != nil || media.GetPredicateName() != mtproto.Predicate_messageMediaPoll {
		t.Fatalf("on %+v %v", media, err)
	}
	if !media.GetPoll().GetQuiz() {
		t.Fatal("quiz flag dropped")
	}
}
