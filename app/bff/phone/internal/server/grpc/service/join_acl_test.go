package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/engine"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestJoinGroupCallRoomACL is phone.joinGroupCall against the chat that owns
// the call. A private supergroup admits members and admins, and admits a
// non-member only with the hash from phone.exportGroupCallInvite. A public
// supergroup admits a non-member with no hash and with a wrong hash.
// join_as is checked on its own: another user, or a channel the caller is
// not in, is refused even when the caller may enter the call.
func TestJoinGroupCallRoomACL(t *testing.T) {
	prev := channel.Shared()
	ch := channel.New(channel.NewMemoryStore())
	channel.Use(ch)
	t.Cleanup(func() { channel.Use(prev) })

	ctx := withUser(10, 100)
	created, err := ch.Create(ctx, channel.CreateRequest{CreatorID: 10, Title: "Private", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	room := created.Chats[0]
	roomIn := inputChannel(room)
	if _, err = ch.Invite(ctx, 10, 0, roomIn, []int64{11, 12}, nil); err != nil {
		t.Fatal(err)
	}
	rights := mtproto.MakeTLChatAdminRights(&mtproto.ChatAdminRights{ManageCall: true}).To_ChatAdminRights()
	if _, err = ch.EditAdmin(ctx, 10, 0, roomIn, 12, rights, "host"); err != nil {
		t.Fatal(err)
	}
	alias, err := ch.Create(withUser(11, 110), channel.CreateRequest{CreatorID: 11, Title: "Alias", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	outsiderCh, err := ch.Create(withUser(14, 140), channel.CreateRequest{CreatorID: 14, Title: "Outsider", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}

	eng := engine.New(engine.Options{
		Config: config.Config{PublicIP: "203.0.113.10", SFUPort: 3480}.Normalized(),
		Now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
	})
	svc := NewWithEngine(eng)
	peer := peerChannel(room)
	up, err := svc.PhoneCreateGroupCall(ctx, &mtproto.TLPhoneCreateGroupCall{Peer: peer, RandomId: 1, Title: wrapperspb.String("room")})
	if err != nil {
		t.Fatal(err)
	}
	call := groupCallInput(t, up)

	// A stale hash must not lock a member or an admin out.
	memberJoin, err := joinCall(svc, 11, 110, call, selfPeer(), "wrong-hash", 501, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := joinedSelf(t, memberJoin); got.GetPeer().GetUserId() != 11 || !got.GetMuted() || got.GetCanSelfUnmute() {
		t.Fatalf("member %+v", got)
	}
	if _, err = joinCall(svc, 12, 120, call, selfPeer(), "", 502, true); err != nil {
		t.Fatalf("admin: %v", err)
	}
	if _, err = joinCall(svc, 10, 100, call, selfPeer(), "", 503, true); err != nil {
		t.Fatalf("creator: %v", err)
	}

	if _, err = joinCall(svc, 13, 130, call, selfPeer(), "", 504, true); rpcText(err) != "GROUPCALL_FORBIDDEN" {
		t.Fatalf("non-member no hash: %v", err)
	}
	if _, err = joinCall(svc, 13, 130, call, selfPeer(), "wrong-hash", 505, true); rpcText(err) != "GROUPCALL_FORBIDDEN" {
		t.Fatalf("non-member wrong hash: %v", err)
	}
	if _, err = joinCall(svc, 14, 140, call, peerChannel(outsiderCh.Chats[0]), "", 506, true); rpcText(err) != "GROUPCALL_FORBIDDEN" {
		t.Fatalf("non-member join_as own channel: %v", err)
	}
	if _, err = joinCall(svc, 11, 110, call, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 13}).To_InputPeer(), "", 507, true); rpcText(err) != "JOIN_AS_PEER_INVALID" {
		t.Fatalf("join_as another user: %v", err)
	}
	if _, err = joinCall(svc, 11, 110, call, peerChannel(outsiderCh.Chats[0]), "", 508, true); rpcText(err) != "JOIN_AS_PEER_INVALID" {
		t.Fatalf("join_as foreign channel: %v", err)
	}

	view, err := svc.PhoneGetGroupCall(ctx, &mtproto.TLPhoneGetGroupCall{Call: call, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	users, channels := participantPeers(view)
	for _, id := range []int64{10, 11, 12} {
		if !users[id] {
			t.Fatalf("participant %d missing from %v", id, users)
		}
	}
	if users[13] || users[14] || len(channels) != 0 {
		t.Fatalf("outsider was admitted users=%v channels=%v", users, channels)
	}
	if _, err = ch.Invite(ctx, 10, 0, roomIn, []int64{15}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = joinCall(svc, 15, 150, call, selfPeer(), "", 513, true); err != nil {
		t.Fatalf("invited member: %v", err)
	}
	if _, err = ch.Leave(withUser(15, 150), 15, 0, roomIn); err != nil {
		t.Fatal(err)
	}
	if _, err = joinCall(svc, 15, 150, call, selfPeer(), "", 514, true); rpcText(err) != "GROUPCALL_FORBIDDEN" {
		t.Fatalf("member who left: %v", err)
	}

	link, err := svc.PhoneExportGroupCallInvite(ctx, &mtproto.TLPhoneExportGroupCallInvite{Call: call})
	if err != nil {
		t.Fatal(err)
	}
	hash := inviteSlug(t, link.GetLink())
	speakerLink, err := svc.PhoneExportGroupCallInvite(ctx, &mtproto.TLPhoneExportGroupCallInvite{Call: call, CanSelfUnmute: true})
	if err != nil {
		t.Fatal(err)
	}
	speaker := inviteSlug(t, speakerLink.GetLink())
	if speaker == hash {
		t.Fatal("speaker invite matched the listener invite")
	}

	admitted, err := joinCall(svc, 13, 130, call, selfPeer(), hash, 509, true)
	if err != nil {
		t.Fatalf("valid invite: %v", err)
	}
	if got := joinedSelf(t, admitted); got.GetPeer().GetUserId() != 13 || !got.GetMuted() || got.GetCanSelfUnmute() {
		t.Fatalf("listener invite %+v", got)
	}
	asChannel, err := joinCall(svc, 14, 140, call, peerChannel(outsiderCh.Chats[0]), hash, 510, true)
	if err != nil {
		t.Fatalf("valid invite join_as own channel: %v", err)
	}
	if got := joinedSelf(t, asChannel); got.GetPeer().GetChannelId() != outsiderCh.Chats[0].GetId() {
		t.Fatalf("join_as channel %+v", got.GetPeer())
	}
	asAlias, err := joinCall(svc, 11, 110, call, peerChannel(alias.Chats[0]), "", 511, true)
	if err != nil {
		t.Fatalf("member join_as own channel: %v", err)
	}
	if got := joinedSelf(t, asAlias); got.GetPeer().GetChannelId() != alias.Chats[0].GetId() {
		t.Fatalf("alias peer %+v", got.GetPeer())
	}
	spoken, err := joinCall(svc, 18, 180, call, selfPeer(), speaker, 512, false)
	if err != nil {
		t.Fatalf("speaker invite: %v", err)
	}
	if got := joinedSelf(t, spoken); got.GetMuted() || !got.GetCanSelfUnmute() {
		t.Fatalf("speaker rights %+v", got)
	}

	pubCreated, err := ch.Create(ctx, channel.CreateRequest{CreatorID: 10, Title: "Open", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	pub := pubCreated.Chats[0]
	if _, err = ch.UpdateUsername(ctx, 10, 0, inputChannel(pub), "callroompub"); err != nil {
		t.Fatal(err)
	}
	pubUp, err := svc.PhoneCreateGroupCall(ctx, &mtproto.TLPhoneCreateGroupCall{
		Peer: peerChannel(pub), RandomId: 2, Title: wrapperspb.String("open"),
	})
	if err != nil {
		t.Fatal(err)
	}
	pubCall := groupCallInput(t, pubUp)
	if _, err = joinCall(svc, 13, 130, pubCall, selfPeer(), "", 601, true); err != nil {
		t.Fatalf("public non-member: %v", err)
	}
	if _, err = joinCall(svc, 19, 190, pubCall, selfPeer(), "wrong-hash", 602, true); err != nil {
		t.Fatalf("public non-member wrong hash: %v", err)
	}
}

func joinCall(svc *Service, user, auth int64, call *mtproto.InputGroupCall, as *mtproto.InputPeer, hash string, ssrc int, muted bool) (*mtproto.Updates, error) {
	in := &mtproto.TLPhoneJoinGroupCall{
		Muted:  muted,
		Call:   call,
		JoinAs: as,
		Params: mtproto.MakeTLDataJSON(&mtproto.DataJSON{
			Data: fmt.Sprintf(`{"ssrc":%d,"ufrag":"cu","pwd":"cp","fingerprints":[{"hash":"sha-256","fingerprint":"AA:BB","setup":"passive"}]}`, ssrc),
		}).To_DataJSON(),
	}
	if hash != "" {
		in.InviteHash = wrapperspb.String(hash)
	}
	return svc.PhoneJoinGroupCall(withUser(user, auth), in)
}

func groupCallInput(t *testing.T, up *mtproto.Updates) *mtproto.InputGroupCall {
	t.Helper()
	for _, u := range up.GetUpdates() {
		if g := u.GetCall_GROUPCALL(); g != nil && g.GetId() != 0 {
			return mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: g.GetId(), AccessHash: g.GetAccessHash()}).To_InputGroupCall()
		}
	}
	t.Fatalf("no group call in %+v", up)
	return nil
}

func joinedSelf(t *testing.T, up *mtproto.Updates) *mtproto.GroupCallParticipant {
	t.Helper()
	for _, u := range up.GetUpdates() {
		if u.GetPredicateName() != mtproto.Predicate_updateGroupCallParticipants {
			continue
		}
		for _, p := range u.GetParticipants_VECTORGROUPCALLPARTICIPANT() {
			if p.GetSelf() {
				return p
			}
		}
	}
	t.Fatal("self participant missing")
	return nil
}

func participantPeers(view *mtproto.Phone_GroupCall) (users, channels map[int64]bool) {
	users = map[int64]bool{}
	channels = map[int64]bool{}
	for _, p := range view.GetParticipants() {
		peer := p.GetPeer()
		if peer == nil {
			continue
		}
		switch peer.GetPredicateName() {
		case mtproto.Predicate_peerUser:
			users[peer.GetUserId()] = true
		case mtproto.Predicate_peerChannel:
			channels[peer.GetChannelId()] = true
		}
	}
	return users, channels
}

func peerChannel(ch *mtproto.Chat) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId:  ch.GetId(),
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputPeer()
}

func selfPeer() *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerSelf(nil).To_InputPeer()
}

func inviteSlug(t *testing.T, link string) string {
	t.Helper()
	i := strings.LastIndex(link, "/")
	if i < 0 || i == len(link)-1 {
		t.Fatalf("invite link %q", link)
	}
	return link[i+1:]
}

func rpcText(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}
