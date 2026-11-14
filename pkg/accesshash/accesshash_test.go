package accesshash

import (
	"context"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

func TestParseMode(t *testing.T) {
	cases := map[string]Mode{
		"":        ModeOff,
		"off":     ModeOff,
		"0":       ModeOff,
		"false":   ModeOff,
		"log":     ModeLog,
		"LOG":     ModeLog,
		"1":       ModeEnforce,
		"true":    ModeEnforce,
		"on":      ModeEnforce,
		"enforce": ModeEnforce,
		"maybe":   ModeOff,
	}
	for in, want := range cases {
		if got := ParseMode(in); got != want {
			t.Fatalf("%q -> %d, want %d", in, got, want)
		}
	}
}

func TestAcceptIssuedHashOnly(t *testing.T) {
	if !Accept(99, []int64{99}) || Accept(0, []int64{99}) || Accept(1, []int64{99}) {
		t.Fatal("issued hash 99")
	}
	if Accept(0, nil) || Accept(5, []int64{}) {
		t.Fatal("missing user must not match")
	}
	if !Accept(7, []int64{3, 7}) || Accept(4, []int64{3, 7}) {
		t.Fatal("any issued hash")
	}
	// A row that really stores 0 matches only 0. Seeded rows do not.
	if !Accept(0, []int64{0}) || Accept(1, []int64{0}) {
		t.Fatal("stored zero")
	}
}

func TestAllowProvidedKeepsZeroSkipUntilEnforce(t *testing.T) {
	defer SetMode(ModeOff)()
	if !AllowProvided(0, 99) || !AllowProvided(99, 0) || AllowProvided(1, 2) {
		t.Fatal("off")
	}
	defer SetMode(ModeEnforce)()
	if AllowProvided(0, 99) || !AllowProvided(99, 99) || AllowProvided(1, 99) {
		t.Fatal("enforce")
	}
}

type memBook struct {
	users    map[int64][]int64
	channels map[int64][]int64
	userHit  bool
	msgUser  func(peer *mtproto.InputPeer, msgID int32, userID int64) bool
	msgChan  func(peer *mtproto.InputPeer, msgID int32, channelID int64) bool
	err      error
}

func (m *memBook) UserHashes(ctx context.Context, viewer int64, ids []int64) (map[int64][]int64, error) {
	m.userHit = true
	if m.err != nil {
		return nil, m.err
	}
	out := map[int64][]int64{}
	for _, id := range ids {
		out[id] = m.users[id]
	}
	return out, nil
}

func (m *memBook) ChannelHashes(ctx context.Context, ids []int64) (map[int64][]int64, error) {
	if m.err != nil {
		return nil, m.err
	}
	out := map[int64][]int64{}
	for _, id := range ids {
		out[id] = m.channels[id]
	}
	return out, nil
}

func (m *memBook) MessageHasUser(ctx context.Context, viewer int64, peer *mtproto.InputPeer, msgID int32, userID int64) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	if m.msgUser == nil {
		return false, nil
	}
	return m.msgUser(peer, msgID, userID), nil
}

func (m *memBook) MessageHasChannel(ctx context.Context, viewer int64, peer *mtproto.InputPeer, msgID int32, channelID int64) (bool, error) {
	if m.err != nil {
		return false, m.err
	}
	if m.msgChan == nil {
		return false, nil
	}
	return m.msgChan(peer, msgID, channelID), nil
}

func code(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func userIn(id, hash int64) *mtproto.InputUser {
	return mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: id, AccessHash: hash}).To_InputUser()
}

func peerUser(id, hash int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: id, AccessHash: hash}).To_InputPeer()
}

func TestOffDoesNotConsultTheBook(t *testing.T) {
	defer SetMode(ModeOff)()
	book := &memBook{err: context.Canceled}
	req := &mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{userIn(2, 0)}}
	if err := Check(context.Background(), 1, "users.getUsers", req, book, nil); err != nil {
		t.Fatal(err)
	}
	if book.userHit {
		t.Fatal("off mode looked up a hash")
	}
}

func TestEnumerationBySequentialUserID(t *testing.T) {
	defer SetMode(ModeEnforce)()
	users := map[int64][]int64{}
	for id := int64(1); id <= 5; id++ {
		users[id] = []int64{1000 + id}
	}
	book := &memBook{users: users}
	var logs []string
	for id := int64(1); id <= 5; id++ {
		req := &mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{userIn(id, 0)}}
		err := Check(context.Background(), 9, "/mtproto.RPCUsers/users_getUsers", req, book, func(s string) {
			logs = append(logs, s)
		})
		if code(err) != "USER_ID_INVALID" {
			t.Fatalf("id %d: %v", id, err)
		}
		// A wrong non-zero guess is the same error. The profile is not returned.
		req = &mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{userIn(id, 1)}}
		if code(Check(context.Background(), 9, "users.getUsers", req, book, nil)) != "USER_ID_INVALID" {
			t.Fatalf("id %d wrong hash", id)
		}
	}
	if len(logs) != 5 {
		t.Fatalf("logs %d", len(logs))
	}
	ok := &mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{userIn(3, 1003)}}
	if err := Check(context.Background(), 9, "users.getUsers", ok, book, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSelfAndChatAreExempt(t *testing.T) {
	defer SetMode(ModeEnforce)()
	book := &memBook{err: context.Canceled}
	req := &mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{
		mtproto.MakeTLInputUserSelf(nil).To_InputUser(),
	}}
	if err := Check(context.Background(), 9, "users.getUsers", req, book, nil); err != nil {
		t.Fatal(err)
	}
	send := &mtproto.TLMessagesSendMessage{
		Peer: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	}
	if err := Check(context.Background(), 9, "messages.sendMessage", send, book, nil); err != nil {
		t.Fatal(err)
	}
	send.Peer = mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 4}).To_InputPeer()
	if err := Check(context.Background(), 9, "messages.sendMessage", send, book, nil); err != nil {
		t.Fatal(err)
	}
	if book.userHit {
		t.Fatal("exempt constructors looked up a hash")
	}
}

func TestConstructorsAndBareIDs(t *testing.T) {
	defer SetMode(ModeEnforce)()
	book := &memBook{users: map[int64][]int64{2: {9}}, channels: map[int64][]int64{8: {70}}}

	photos := &mtproto.TLPhotosGetUserPhotos{UserId: userIn(2, 0)}
	if code(Check(context.Background(), 1, "photos.getUserPhotos", photos, book, nil)) != "USER_ID_INVALID" {
		t.Fatal("photos")
	}
	photos.UserId = userIn(2, 9)
	if err := Check(context.Background(), 1, "photos.getUserPhotos", photos, book, nil); err != nil {
		t.Fatal(err)
	}

	contacts := &mtproto.TLContactsDeleteContacts{Id: []*mtproto.InputUser{
		{Constructor: mtproto.CRC32_inputUser, UserId: 2, AccessHash: 0},
	}}
	if code(Check(context.Background(), 1, "contacts.deleteContacts", contacts, book, nil)) != "USER_ID_INVALID" {
		t.Fatal("bare constructor")
	}
	contacts.Id[0].AccessHash = 9
	if err := Check(context.Background(), 1, "contacts.deleteContacts", contacts, book, nil); err != nil {
		t.Fatal(err)
	}

	bare := &mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{{UserId: 2}}}
	if code(Check(context.Background(), 1, "users.getUsers", bare, book, nil)) != "USER_ID_INVALID" {
		t.Fatal("id only")
	}

	send := &mtproto.TLMessagesSendMessage{Peer: peerUser(2, 1)}
	if code(Check(context.Background(), 1, "messages.sendMessage", send, book, nil)) != "PEER_ID_INVALID" {
		t.Fatal("send")
	}
	send.Peer = peerUser(2, 9)
	if err := Check(context.Background(), 1, "messages.sendMessage", send, book, nil); err != nil {
		t.Fatal(err)
	}

	chs := &mtproto.TLChannelsGetChannels{Id: []*mtproto.InputChannel{
		mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 8, AccessHash: 0}).To_InputChannel(),
	}}
	if code(Check(context.Background(), 1, "channels.getChannels", chs, book, nil)) != "CHANNEL_INVALID" {
		t.Fatal("channel")
	}
	chs.Id[0] = mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 8, AccessHash: 70}).To_InputChannel()
	if err := Check(context.Background(), 1, "channels.getChannels", chs, book, nil); err != nil {
		t.Fatal(err)
	}

	peerCh := &mtproto.TLMessagesSendMessage{Peer: mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: 8, AccessHash: 1,
	}).To_InputPeer()}
	if code(Check(context.Background(), 1, "messages.sendMessage", peerCh, book, nil)) != "PEER_ID_INVALID" {
		t.Fatal("peer channel")
	}

	inline := &mtproto.TLMessagesGetInlineBotResults{
		Bot:  userIn(2, 9),
		Peer: peerUser(2, 9),
	}
	if err := Check(context.Background(), 1, "messages.getInlineBotResults", inline, book, nil); err != nil {
		t.Fatal(err)
	}
	inline.Bot = userIn(2, 0)
	if code(Check(context.Background(), 1, "messages.getInlineBotResults", inline, book, nil)) != "USER_ID_INVALID" {
		t.Fatal("inline bot")
	}
}

func TestLogRecordsAndAllows(t *testing.T) {
	defer SetMode(ModeLog)()
	book := &memBook{users: map[int64][]int64{2: {9}}}
	var line string
	req := &mtproto.TLUsersGetFullUser{Id: userIn(2, 0)}
	if err := Check(context.Background(), 4, "users.getFullUser", req, book, func(s string) { line = s }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(line, "kind=inputUser") || !strings.Contains(line, "target=2") || !strings.Contains(line, "zero=true") {
		t.Fatalf("log %q", line)
	}
}

func TestFromMessageProof(t *testing.T) {
	defer SetMode(ModeEnforce)()
	book := &memBook{
		users: map[int64][]int64{8: {11}},
		msgUser: func(peer *mtproto.InputPeer, msgID int32, userID int64) bool {
			return peer.GetUserId() == 8 && msgID == 4 && userID == 2
		},
	}
	good := &mtproto.TLUsersGetFullUser{Id: mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{
		UserId: 2,
		MsgId:  4,
		Peer:   peerUser(8, 11),
	}).To_InputUser()}
	if err := Check(context.Background(), 1, "users.getFullUser", good, book, nil); err != nil {
		t.Fatal(err)
	}
	badMsg := &mtproto.TLUsersGetFullUser{Id: mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{
		UserId: 3,
		MsgId:  4,
		Peer:   peerUser(8, 11),
	}).To_InputUser()}
	if code(Check(context.Background(), 1, "users.getFullUser", badMsg, book, nil)) != "USER_ID_INVALID" {
		t.Fatal("user not in the message")
	}
	badPeer := &mtproto.TLUsersGetFullUser{Id: mtproto.MakeTLInputUserFromMessage(&mtproto.InputUser{
		UserId: 2,
		MsgId:  4,
		Peer:   peerUser(8, 0),
	}).To_InputUser()}
	if code(Check(context.Background(), 1, "users.getFullUser", badPeer, book, nil)) != "PEER_ID_INVALID" {
		t.Fatal("container hash")
	}

	send := &mtproto.TLMessagesSendMessage{Peer: mtproto.MakeTLInputPeerUserFromMessage(&mtproto.InputPeer{
		UserId: 2,
		MsgId:  4,
		Peer:   peerUser(8, 11),
	}).To_InputPeer()}
	if err := Check(context.Background(), 1, "messages.sendMessage", send, book, nil); err != nil {
		t.Fatal(err)
	}
	send.Peer = mtproto.MakeTLInputPeerUserFromMessage(&mtproto.InputPeer{
		UserId: 9,
		MsgId:  4,
		Peer:   peerUser(8, 11),
	}).To_InputPeer()
	if code(Check(context.Background(), 1, "messages.sendMessage", send, book, nil)) != "PEER_ID_INVALID" {
		t.Fatal("peer from message")
	}
}

func TestFromMessageChannel(t *testing.T) {
	defer SetMode(ModeEnforce)()
	book := &memBook{
		channels: map[int64][]int64{5: {50}},
		msgChan: func(peer *mtproto.InputPeer, msgID int32, channelID int64) bool {
			return peer.GetChatId() == 3 && msgID == 6 && channelID == 5
		},
	}
	req := &mtproto.TLMessagesSendMessage{Peer: mtproto.MakeTLInputPeerChannelFromMessage(&mtproto.InputPeer{
		ChannelId: 5,
		MsgId:     6,
		Peer:      mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 3}).To_InputPeer(),
	}).To_InputPeer()}
	if err := Check(context.Background(), 1, "messages.sendMessage", req, book, nil); err != nil {
		t.Fatal(err)
	}
	req.Peer = mtproto.MakeTLInputPeerChannelFromMessage(&mtproto.InputPeer{
		ChannelId: 5,
		MsgId:     7,
		Peer:      mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 3}).To_InputPeer(),
	}).To_InputPeer()
	if code(Check(context.Background(), 1, "messages.sendMessage", req, book, nil)) != "PEER_ID_INVALID" {
		t.Fatal("channel not in message")
	}
	in := &mtproto.TLChannelsGetChannels{Id: []*mtproto.InputChannel{
		mtproto.MakeTLInputChannelFromMessage(&mtproto.InputChannel{
			ChannelId: 9,
			MsgId:     6,
			Peer:      mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 3}).To_InputPeer(),
		}).To_InputChannel(),
	}}
	if code(Check(context.Background(), 1, "channels.getChannels", in, book, nil)) != "CHANNEL_INVALID" {
		t.Fatal("inputChannelFromMessage")
	}
}

func TestStoreFailureFailsClosedOnlyWhenEnforcing(t *testing.T) {
	book := &memBook{err: context.Canceled}
	req := &mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{userIn(2, 1)}}
	defer SetMode(ModeLog)()
	if err := Check(context.Background(), 1, "users.getUsers", req, book, func(string) {}); err != nil {
		t.Fatal(err)
	}
	defer SetMode(ModeEnforce)()
	if code(Check(context.Background(), 1, "users.getUsers", req, book, func(string) {})) != "INTERNAL_SERVER_ERROR" {
		t.Fatal("enforce should fail closed")
	}
}

func TestUnauthenticatedSkipped(t *testing.T) {
	defer SetMode(ModeEnforce)()
	book := &memBook{err: context.Canceled}
	req := &mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{userIn(2, 0)}}
	if err := Check(context.Background(), 0, "users.getUsers", req, book, nil); err != nil {
		t.Fatal(err)
	}
}

func TestBatchFailsOnOneBadHash(t *testing.T) {
	defer SetMode(ModeEnforce)()
	book := &memBook{users: map[int64][]int64{2: {9}, 3: {8}}}
	req := &mtproto.TLUsersGetUsers{Id: []*mtproto.InputUser{userIn(2, 9), userIn(3, 0)}}
	if code(Check(context.Background(), 1, "users.getUsers", req, book, nil)) != "USER_ID_INVALID" {
		t.Fatal("batch")
	}
}

func TestMessageContainsUserAndChannel(t *testing.T) {
	msg := &mtproto.Message{
		PredicateName: mtproto.Predicate_message,
		FromId:        mtproto.MakePeerUser(2),
		ViaBotId:      mtproto.MakeFlagsInt64(3),
		Entities: []*mtproto.MessageEntity{{
			PredicateName: mtproto.Predicate_messageEntityMentionName,
			UserId_INT64:  4,
		}},
		Media:  &mtproto.MessageMedia{UserId: 5},
		Action: &mtproto.MessageAction{Users: []int64{6}},
		FwdFrom: mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
			FromId: mtproto.MakePeerUser(7),
		}).To_MessageFwdHeader(),
	}
	for _, id := range []int64{2, 3, 4, 5, 6, 7} {
		if !MessageContainsUser(msg, 0, id) {
			t.Fatalf("user %d", id)
		}
	}
	if MessageContainsUser(msg, 0, 8) {
		t.Fatal("stranger")
	}
	if !MessageContainsUser(&mtproto.Message{PredicateName: mtproto.Predicate_message}, 2, 2) {
		t.Fatal("private sender")
	}
	empty := &mtproto.Message{PredicateName: mtproto.Predicate_messageEmpty}
	if MessageContainsUser(empty, 2, 2) {
		t.Fatal("deleted message")
	}

	fwd := &mtproto.Message{
		PredicateName: mtproto.Predicate_message,
		FwdFrom: mtproto.MakeTLMessageFwdHeader(&mtproto.MessageFwdHeader{
			FromId: mtproto.MakePeerChannel(40),
		}).To_MessageFwdHeader(),
	}
	if !MessageContainsChannel(fwd, 40) || MessageContainsChannel(fwd, 41) {
		t.Fatal("forwarded channel")
	}
	if MessageContainsChannel(&mtproto.Message{
		PredicateName: mtproto.Predicate_message,
		PeerId:        mtproto.MakePeerChannel(40),
	}, 40) {
		t.Fatal("the dialog itself is not a reference")
	}
}

var _ proto.Message = (*mtproto.TLUsersGetUsers)(nil)
