package service

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/engine"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestAllowToSpeakAndLocalMute(t *testing.T) {
	ch := channel.New(channel.NewMemoryStore())
	prev := channel.Shared()
	channel.Use(ch)
	t.Cleanup(func() { channel.Use(prev) })
	created, err := ch.Create(withUser(10, 1), channel.CreateRequest{CreatorID: 10, Title: "Voice", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ch.Invite(withUser(10, 1), 10, 0, inputChannel(created.Chats[0]), []int64{11}, nil); err != nil {
		t.Fatal(err)
	}
	chat := created.Chats[0]
	eng := engine.New(engine.Options{
		Config: config.Config{PublicIP: "203.0.113.10", SFUPort: 3480, CallLinkPrefix: "https://tell.cc/call/"}.Normalized(),
		Now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
		Members: func(peerType int32, peerID int64) []int64 {
			ids, _ := channel.Shared().MemberIDs(withUser(10, 0), peerID)
			return ids
		},
	})
	svc := NewWithEngine(eng)
	ctx := withUser(10, 100)
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: chat.Id, AccessHash: chat.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputPeer()
	up, err := svc.PhoneCreateGroupCall(ctx, &mtproto.TLPhoneCreateGroupCall{Peer: peer, RandomId: 8})
	if err != nil {
		t.Fatal(err)
	}
	gcall := up.Updates[0].GetCall_GROUPCALL()
	inCall := mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: gcall.GetId(), AccessHash: gcall.GetAccessHash()}).To_InputGroupCall()
	params := `{"ssrc":222,"ufrag":"cu","pwd":"cp","fingerprints":[{"hash":"sha-256","fingerprint":"AA:BB","setup":"passive"}]}`
	if _, err = svc.PhoneJoinGroupCall(ctx, &mtproto.TLPhoneJoinGroupCall{
		Call: inCall, JoinAs: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Params: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: params}).To_DataJSON(),
	}); err != nil {
		t.Fatal(err)
	}
	memberParams := `{"ssrc":333,"ufrag":"mu","pwd":"mp","fingerprints":[{"hash":"sha-256","fingerprint":"AA:BB","setup":"passive"}]}`
	joined, err := svc.PhoneJoinGroupCall(withUser(11, 110), &mtproto.TLPhoneJoinGroupCall{
		Muted: true, Call: inCall, JoinAs: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Params: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: memberParams}).To_DataJSON(),
	})
	if err != nil {
		t.Fatal(err)
	}
	selfPart := joinedSelf(t, joined)
	if !selfPart.GetMuted() || !selfPart.GetCanSelfUnmute() {
		t.Fatalf("member joined forced mute %+v", selfPart)
	}
	if _, err = svc.PhoneEditGroupCallParticipant(withUser(11, 110), &mtproto.TLPhoneEditGroupCallParticipant{
		Call: inCall, Participant: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(), RaiseHand: mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	memberPeer := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 11}).To_InputPeer()
	if _, err = svc.PhoneEditGroupCallParticipant(ctx, &mtproto.TLPhoneEditGroupCallParticipant{
		Call: inCall, Participant: memberPeer, Muted: mtproto.BoolFalse,
	}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.PhoneGetGroupCall(withUser(11, 110), &mtproto.TLPhoneGetGroupCall{Call: inCall, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var member *mtproto.GroupCallParticipant
	for _, p := range view.GetParticipants() {
		if p.GetPeer().GetUserId() == 11 {
			member = p
		}
	}
	if member == nil || !member.GetMuted() || !member.GetCanSelfUnmute() || member.GetRaiseHandRating() != nil {
		t.Fatalf("allow to speak %+v", member)
	}
	vol := int32(40)
	if _, err = svc.PhoneEditGroupCallParticipant(withUser(11, 110), &mtproto.TLPhoneEditGroupCallParticipant{
		Call: inCall, Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 10}).To_InputPeer(),
		Volume: wrapperspb.Int32(vol), Muted: mtproto.BoolTrue,
	}); err != nil {
		t.Fatalf("local mute: %v", err)
	}
	mine, err := svc.PhoneGetGroupCall(withUser(11, 110), &mtproto.TLPhoneGetGroupCall{Call: inCall, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	var host *mtproto.GroupCallParticipant
	for _, p := range mine.GetParticipants() {
		if p.GetPeer().GetUserId() == 10 {
			host = p
		}
	}
	if host == nil || !host.GetMutedByYou() || host.GetVolume().GetValue() != vol || host.GetVolumeByAdmin() {
		t.Fatalf("listener view %+v", host)
	}
	adminView, err := svc.PhoneGetGroupCall(ctx, &mtproto.TLPhoneGetGroupCall{Call: inCall, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range adminView.GetParticipants() {
		if p.GetPeer().GetUserId() == 10 && (p.GetMutedByYou() || p.GetVolumeByAdmin()) {
			t.Fatalf("admin saw a private mute %+v", p)
		}
	}
}
