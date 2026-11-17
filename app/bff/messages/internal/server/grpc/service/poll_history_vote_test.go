package service

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/core"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/messages/internal/svc"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	message_client "github.com/teamgram/teamgram-server/app/service/biz/message/client"
	"github.com/teamgram/teamgram-server/app/service/biz/message/message"
	user_client "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/zeromicro/go-zero/core/jsonx"
)

// TestPollSendHistoryLayer232AndVote covers ISSUE-S-POLL-001 row 70 with
// TELLOMI_POLLS=1 in this process only. Poll media is the object
// messages.sendMedia stores (see TestSendMediaBuildsLayer232Poll). Each
// side's message is stored and reloaded with jsonx, which is the
// message_data column. messages.getHistory, messages.sendVote, and
// messages.getPollVotes are the real handlers. The message client only
// returns those reloaded rows: Kafka, MySQL, and the MTProto socket are
// not running here.
func TestPollSendHistoryLayer232AndVote(t *testing.T) {
	t.Setenv("TELLOMI_POLLS", "1")
	chatbox.ResetShared()
	t.Cleanup(chatbox.ResetShared)

	// messages.sendMedia reaches this through makeMediaByInputMedia. That
	// method is unexported in package core; TestSendMediaBuildsLayer232Poll
	// calls it. This handler test starts from the poll media that call returns.
	media, err := chatbox.PollInputMedia(context.Background(), 1, inputFixturePoll())
	if err != nil {
		t.Fatal(err)
	}
	if media.GetPredicateName() != mtproto.Predicate_messageMediaPoll {
		t.Fatalf("send media %s", media.GetPredicateName())
	}
	pollID := media.GetPoll().GetId()
	if pollID == 0 || !media.GetPoll().GetPublicVoters() {
		t.Fatalf("poll %+v", media.GetPoll())
	}

	senderMsg := reloadMessage(t, mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		Id:      11,
		FromId:  mtproto.MakePeerUser(1),
		PeerId:  mtproto.MakePeerUser(2),
		Date:    1_700_000_000,
		Message: "",
		Media:   media,
	}).To_Message())
	receiverMsg := reloadMessage(t, mtproto.MakeTLMessage(&mtproto.Message{
		Out:     false,
		Id:      44,
		FromId:  mtproto.MakePeerUser(1),
		PeerId:  mtproto.MakePeerUser(1),
		Date:    1_700_000_000,
		Message: "",
		Media:   media,
	}).To_Message())

	boxes := map[int64][]*mtproto.MessageBox{
		1: {{
			UserId: 1, MessageId: 11, SenderUserId: 1,
			PeerType: mtproto.PEER_USER, PeerId: 2, Message: senderMsg,
		}},
		2: {{
			UserId: 2, MessageId: 44, SenderUserId: 1,
			PeerType: mtproto.PEER_USER, PeerId: 1, Message: receiverMsg,
		}},
	}
	byID := map[int64]map[int32]*mtproto.MessageBox{
		1: {11: boxes[1][0]},
		2: {44: boxes[2][0]},
	}

	for _, viewer := range []struct {
		user int64
		peer int64
	}{{1, 2}, {2, 1}} {
		page := history(t, viewer.user, viewer.peer, boxes)
		if len(page.GetMessages()) != 1 {
			t.Fatalf("user %d history %+v", viewer.user, page)
		}
		assertLayer232Poll(t, page.GetMessages()[0], pollID)
	}

	svcPoll := PollsService{Messages: pollBoxStore{boxes: byID}}
	voted, err := svcPoll.MessagesSendVote(withUser(2), &mtproto.TLMessagesSendVote{
		Peer:    mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 1}).To_InputPeer(),
		MsgId:   44,
		Options: [][]byte{[]byte("0")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if voted.Updates[0].GetResults().GetTotalVoters().GetValue() != 1 {
		t.Fatalf("vote totals %+v", voted.Updates[0].GetResults())
	}
	if !chosenOption(voted.Updates[0].GetResults(), "0") {
		t.Fatalf("chosen %+v", voted.Updates[0].GetResults().GetResults())
	}

	after := history(t, 2, 1, boxes)
	assertLayer232Poll(t, after.GetMessages()[0], pollID)
	if after.GetMessages()[0].GetMedia().GetResults().GetTotalVoters().GetValue() != 1 {
		t.Fatalf("history totals %+v", after.GetMessages()[0].GetMedia().GetResults())
	}

	list, err := svcPoll.MessagesGetPollVotes(withUser(1), &mtproto.TLMessagesGetPollVotes{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 2}).To_InputPeer(),
		Id:    11,
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if list.GetCount() != 1 || len(list.GetUsers()) != 1 || list.GetUsers()[0].GetId() != 2 {
		t.Fatalf("voters %+v", list)
	}
	votes := list.GetVotes_VECTORMESSAGEPEERVOTE()
	if len(votes) != 1 || string(votes[0].GetOption()) != "0" {
		t.Fatalf("vote option %+v", votes)
	}

	switched, err := svcPoll.MessagesSendVote(withUser(2), &mtproto.TLMessagesSendVote{
		Peer:    mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 1}).To_InputPeer(),
		MsgId:   44,
		Options: [][]byte{[]byte("1")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !chosenOption(switched.Updates[0].GetResults(), "1") || chosenOption(switched.Updates[0].GetResults(), "0") {
		t.Fatalf("switch %+v", switched.Updates[0].GetResults().GetResults())
	}
	if switched.Updates[0].GetResults().GetTotalVoters().GetValue() != 1 {
		t.Fatalf("switch totals %+v", switched.Updates[0].GetResults())
	}

	if _, err = svcPoll.MessagesSendVote(withUser(2), &mtproto.TLMessagesSendVote{
		Peer:    mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 1}).To_InputPeer(),
		MsgId:   44,
		Options: [][]byte{[]byte("0"), []byte("1")},
	}); err != mtproto.ErrOptionInvalid {
		t.Fatalf("multiple on a single-choice poll: %v", err)
	}
}

func inputFixturePoll() *mtproto.InputMedia {
	return mtproto.MakeTLInputMediaPoll(&mtproto.InputMedia{
		Poll: mtproto.MakeTLPoll(&mtproto.Poll{
			PublicVoters: true,
			Question:     "fixture-q",
			Answers: []*mtproto.PollAnswer{
				mtproto.MakePollAnswerWithText("yes", []byte("0")),
				mtproto.MakePollAnswerWithText("no", []byte("1")),
			},
		}).To_Poll(),
	}).To_InputMedia()
}

func reloadMessage(t *testing.T, msg *mtproto.Message) *mtproto.Message {
	t.Helper()
	raw, err := jsonx.Marshal(msg)
	if err != nil {
		t.Fatal(err)
	}
	var back *mtproto.Message
	if err = jsonx.UnmarshalFromString(string(raw), &back); err != nil {
		t.Fatal(err)
	}
	if back.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaPoll {
		t.Fatalf("message_data media %s", back.GetMedia().GetPredicateName())
	}
	return back
}

func history(t *testing.T, user, peer int64, boxes map[int64][]*mtproto.MessageBox) *mtproto.Messages_Messages {
	t.Helper()
	c := core.New(withUser(user), &svc.ServiceContext{Dao: &dao.Dao{
		MessageClient: pollHistory{boxes: boxes},
		UserClient:    pollUsers{},
	}})
	page, err := c.MessagesGetHistory(&mtproto.TLMessagesGetHistory{
		Peer:  mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: peer}).To_InputPeer(),
		Limit: 20,
	})
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func assertLayer232Poll(t *testing.T, msg *mtproto.Message, pollID int64) {
	t.Helper()
	media := msg.GetMedia()
	if media.GetPredicateName() != mtproto.Predicate_messageMediaPoll || media.GetPoll().GetId() != pollID {
		t.Fatalf("history media %+v", media)
	}
	if media.GetPoll().GetQuestion_TEXTWITHENTITIES().GetText() != "fixture-q" {
		t.Fatalf("question %+v", media.GetPoll())
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := msg.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	var pollCtor, unsupported [4]byte
	binary.LittleEndian.PutUint32(pollCtor[:], 0x773f4e66)
	binary.LittleEndian.PutUint32(unsupported[:], 0x9f84f49e)
	if !bytes.Contains(raw, pollCtor[:]) {
		t.Fatal("layer 232 history is missing messageMediaPoll#773f4e66")
	}
	if bytes.Contains(raw, unsupported[:]) {
		t.Fatal("layer 232 history used messageMediaUnsupported#9f84f49e")
	}
	got := &mtproto.Message{}
	if err := got.Decode(mtproto.NewDecodeBuf(raw)); err != nil {
		t.Fatal(err)
	}
	if got.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaPoll {
		t.Fatalf("decoded %s", got.GetMedia().GetPredicateName())
	}
}

func chosenOption(res *mtproto.PollResults, option string) bool {
	for _, row := range res.GetResults() {
		if string(row.GetOption()) == option && row.GetChosen() {
			return true
		}
	}
	return false
}

type pollHistory struct {
	message_client.MessageClient
	boxes map[int64][]*mtproto.MessageBox
}

func (h pollHistory) MessageGetHistoryMessages(_ context.Context, in *message.TLMessageGetHistoryMessages) (*message.Vector_MessageBox, error) {
	return &message.Vector_MessageBox{Datas: h.boxes[in.GetUserId()]}, nil
}

func (h pollHistory) MessageGetHistoryMessagesCount(_ context.Context, in *message.TLMessageGetHistoryMessagesCount) (*mtproto.Int32, error) {
	return &mtproto.Int32{V: int32(len(h.boxes[in.GetUserId()]))}, nil
}

type pollUsers struct{ user_client.UserClient }

func (pollUsers) UserGetMutableUsers(context.Context, *userpb.TLUserGetMutableUsers) (*userpb.Vector_ImmutableUser, error) {
	return &userpb.Vector_ImmutableUser{}, nil
}

type pollBoxStore struct {
	boxes map[int64]map[int32]*mtproto.MessageBox
}

func (s pollBoxStore) MessageGetUserMessage(_ context.Context, in *message.TLMessageGetUserMessage) (*mtproto.MessageBox, error) {
	box := s.boxes[in.GetUserId()][in.GetId()]
	if box == nil || box.GetMessageId() == 0 {
		return nil, mtproto.ErrMessageIdInvalid
	}
	return box, nil
}
