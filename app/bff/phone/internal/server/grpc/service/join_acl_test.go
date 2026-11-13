package service

import (
	"context"
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
// supergroup admits a non-member with no hash and with a wrong hash, and
// refuses a user who was kicked or view-banned. join_as is checked on its
// own: another user, or a channel the caller is not in, is refused even
// when the caller may enter the call. invite_link is only present for a
// viewer who may export it.
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
		MayManage: func(viewer int64, peerType int32, peerID int64) bool {
			if peerType != mtproto.PEER_CHANNEL || peerID == 0 {
				return false
			}
			_, manage, err := channel.Shared().CallAccess(context.Background(), viewer, peerID, 0)
			return err == nil && manage
		},
	})
	svc := NewWithEngine(eng)
	peer := peerChannel(room)
	up, err := svc.PhoneCreateGroupCall(ctx, &mtproto.TLPhoneCreateGroupCall{Peer: peer, RandomId: 1, Title: wrapperspb.String("room")})
	if err != nil {
		t.Fatal(err)
	}
	call := groupCallInput(t, up)
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
	if got := callInviteLink(up); !strings.HasSuffix(got, "/"+hash) {
		t.Fatalf("creator create update invite_link %q", got)
	}

	// The listener slug is not on the call object. A non-member who only has
	// the call id cannot read it back from getGroupCall or inviteToGroupCall.
	outsiderView, err := svc.PhoneGetGroupCall(withUser(13, 130), &mtproto.TLPhoneGetGroupCall{Call: call, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if link := outsiderView.GetCall().GetInviteLink().GetValue(); link != "" {
		if _, joinErr := joinCall(svc, 13, 130, call, selfPeer(), inviteSlug(t, link), 500, true); joinErr == nil {
			t.Fatalf("getGroupCall slug %q admitted a non-member", link)
		}
		t.Fatalf("getGroupCall leaked invite link %q", link)
	}
	invited, err := svc.PhoneInviteToGroupCall(withUser(13, 130), &mtproto.TLPhoneInviteToGroupCall{
		Call:  call,
		Users: []*mtproto.InputUser{mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 16}).To_InputUser()},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, link := range callInviteLinks(invited) {
		if _, joinErr := joinCall(svc, 13, 130, call, selfPeer(), inviteSlug(t, link), 500, true); joinErr == nil {
			t.Fatalf("inviteToGroupCall slug %q admitted a non-member", link)
		}
		t.Fatalf("inviteToGroupCall leaked invite link %q", link)
	}

	// A stale hash must not lock a member or an admin out.
	memberJoin, err := joinCall(svc, 11, 110, call, selfPeer(), "wrong-hash", 501, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := joinedSelf(t, memberJoin); got.GetPeer().GetUserId() != 11 || !got.GetMuted() || got.GetCanSelfUnmute() {
		t.Fatalf("member %+v", got)
	}
	if callInviteLink(memberJoin) != "" || strings.Contains(memberJoin.String(), hash) || strings.Contains(memberJoin.String(), speaker) {
		t.Fatal("member join update carried an invite slug")
	}
	adminJoin, err := joinCall(svc, 12, 120, call, selfPeer(), "", 502, true)
	if err != nil {
		t.Fatalf("admin: %v", err)
	}
	if !strings.HasSuffix(callInviteLink(adminJoin), "/"+hash) {
		t.Fatal("manage_call admin did not receive invite_link")
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
	if !strings.HasSuffix(view.GetCall().GetInviteLink().GetValue(), "/"+hash) || view.GetCall().GetParticipantsCount() == 0 {
		t.Fatalf("creator getGroupCall %+v", view.GetCall())
	}
	memberView, err := svc.PhoneGetGroupCall(withUser(11, 110), &mtproto.TLPhoneGetGroupCall{Call: call, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if memberView.GetCall().GetInviteLink().GetValue() != "" || strings.Contains(memberView.String(), hash) || strings.Contains(memberView.String(), speaker) {
		t.Fatal("member getGroupCall carried an invite slug")
	}
	if memberView.GetCall().GetParticipantsCount() == 0 {
		t.Fatal("member getGroupCall hid the room")
	}
	outsiderView, err = svc.PhoneGetGroupCall(withUser(13, 130), &mtproto.TLPhoneGetGroupCall{Call: call, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if leaked := privateRoomLeak(outsiderView, hash, speaker); leaked != "" {
		t.Fatal(leaked)
	}
	outsiderParts, err := svc.PhoneGetGroupParticipants(withUser(13, 130), &mtproto.TLPhoneGetGroupParticipants{Call: call, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if len(outsiderParts.GetParticipants()) != 0 || len(outsiderParts.GetUsers()) != 0 || strings.Contains(outsiderParts.String(), hash) {
		t.Fatalf("private participants %+v", outsiderParts)
	}
	invited, err = svc.PhoneInviteToGroupCall(withUser(13, 130), &mtproto.TLPhoneInviteToGroupCall{
		Call: call,
		Users: []*mtproto.InputUser{
			mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 16}).To_InputUser(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if callInviteLink(invited) != "" || strings.Contains(invited.String(), hash) || strings.Contains(invited.String(), speaker) {
		t.Fatal("inviteToGroupCall carried an invite slug")
	}
	preview, err := svc.PhoneGetGroupCall(withUser(13, 130), &mtproto.TLPhoneGetGroupCall{
		Call:  mtproto.MakeTLInputGroupCallSlug(&mtproto.InputGroupCall{Slug: hash}).To_InputGroupCall(),
		Limit: 50,
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.GetCall().GetInviteLink().GetValue() != "" || strings.Contains(preview.String(), speaker) {
		t.Fatal("slug preview published an invite link")
	}
	if previewUsers, _ := participantPeers(preview); !previewUsers[11] {
		t.Fatal("slug preview hid the room from the invite holder")
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
	pubLink, err := svc.PhoneExportGroupCallInvite(ctx, &mtproto.TLPhoneExportGroupCallInvite{Call: pubCall})
	if err != nil {
		t.Fatal(err)
	}
	pubHash := inviteSlug(t, pubLink.GetLink())
	pubIn := inputChannel(pub)
	if _, err = ch.Invite(ctx, 10, 0, pubIn, []int64{21, 22, 23}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err = ch.Leave(withUser(21, 210), 21, 0, pubIn); err != nil {
		t.Fatal(err)
	}
	if _, err = joinCall(svc, 21, 210, pubCall, selfPeer(), "", 603, true); err != nil {
		t.Fatalf("public member who left: %v", err)
	}
	kick := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		ViewMessages: true,
		UntilDate:    2147483647,
	}).To_ChatBannedRights()
	if _, err = ch.EditBanned(ctx, 10, 0, pubIn, 22, kick); err != nil {
		t.Fatal(err)
	}
	kickedChat := ch.VisibleChat(withUser(22, 220), 22, pub.GetId())
	if kickedChat == nil || !kickedChat.GetLeft() || !kickedChat.GetBannedRights().GetViewMessages() {
		t.Fatalf("kicked chat %+v", kickedChat)
	}
	if _, err = joinCall(svc, 22, 220, pubCall, selfPeer(), "", 604, true); !groupCallForbidden(err) {
		t.Fatalf("kicked public user: %v", err)
	}
	if _, err = joinCall(svc, 22, 220, pubCall, selfPeer(), pubHash, 605, true); !groupCallForbidden(err) {
		t.Fatalf("kicked public user with invite: %v", err)
	}
	kickedView, err := svc.PhoneGetGroupCall(withUser(22, 220), &mtproto.TLPhoneGetGroupCall{Call: pubCall, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if leaked := privateRoomLeak(kickedView, pubHash, speaker); leaked != "" {
		t.Fatal(leaked)
	}
	viewBan := mtproto.MakeTLChatBannedRights(&mtproto.ChatBannedRights{
		ViewMessages: true,
		UntilDate:    2147483646,
	}).To_ChatBannedRights()
	if _, err = ch.EditBanned(ctx, 10, 0, pubIn, 23, viewBan); err != nil {
		t.Fatal(err)
	}
	bannedChat := ch.VisibleChat(withUser(23, 230), 23, pub.GetId())
	if bannedChat == nil || bannedChat.GetLeft() || !bannedChat.GetBannedRights().GetViewMessages() {
		t.Fatalf("view-banned chat %+v", bannedChat)
	}
	if _, err = joinCall(svc, 23, 230, pubCall, selfPeer(), "", 606, true); !groupCallForbidden(err) {
		t.Fatalf("view-banned public user: %v", err)
	}
	if _, err = joinCall(svc, 23, 230, pubCall, selfPeer(), pubHash, 607, true); !groupCallForbidden(err) {
		t.Fatalf("view-banned public user with invite: %v", err)
	}
	plainView, err := svc.PhoneGetGroupCall(withUser(30, 300), &mtproto.TLPhoneGetGroupCall{Call: pubCall, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if plainView.GetCall().GetInviteLink().GetValue() != "" || strings.Contains(plainView.String(), pubHash) {
		t.Fatal("public non-member received invite_link")
	}
	if plainView.GetCall().GetParticipantsCount() == 0 {
		t.Fatal("public getGroupCall hid the room")
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

func callInviteLinks(up *mtproto.Updates) []string {
	var out []string
	if up == nil {
		return nil
	}
	for _, u := range up.GetUpdates() {
		if link := u.GetCall_GROUPCALL().GetInviteLink().GetValue(); link != "" {
			out = append(out, link)
		}
	}
	return out
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

func groupCallForbidden(err error) bool {
	st := status.Convert(err)
	return st.Code() == 403 && st.Message() == "GROUPCALL_FORBIDDEN"
}

func callInviteLink(up *mtproto.Updates) string {
	for _, u := range up.GetUpdates() {
		if g := u.GetCall_GROUPCALL(); g != nil {
			return g.GetInviteLink().GetValue()
		}
	}
	return ""
}

// privateRoomLeak reports a phone.getGroupCall payload that still tells a
// non-member how to enter or who is inside.
func privateRoomLeak(view *mtproto.Phone_GroupCall, slugs ...string) string {
	if view == nil || view.GetCall() == nil {
		return "missing call"
	}
	call := view.GetCall()
	if call.GetInviteLink().GetValue() != "" || call.GetTitle().GetValue() != "" || call.GetParticipantsCount() != 0 || call.GetRecordVideoActive() {
		return fmt.Sprintf("call shell %+v", call)
	}
	if len(view.GetParticipants()) != 0 || len(view.GetUsers()) != 0 || len(view.GetChats()) != 0 {
		return fmt.Sprintf("roster participants=%d users=%d chats=%d", len(view.GetParticipants()), len(view.GetUsers()), len(view.GetChats()))
	}
	text := view.String()
	for _, slug := range slugs {
		if slug != "" && strings.Contains(text, slug) {
			return "response contains " + slug
		}
	}
	return ""
}
