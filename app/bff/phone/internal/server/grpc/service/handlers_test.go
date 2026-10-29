package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/dh"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/engine"
	"github.com/teamgram/teamgram-server/app/service/biz/channel"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func callProto() *mtproto.PhoneCallProtocol {
	return mtproto.MakeTLPhoneCallProtocol(&mtproto.PhoneCallProtocol{
		UdpP2P: true, UdpReflector: true, MinLayer: 65, MaxLayer: 92,
		LibraryVersions: []string{"5.0.0", "13.0.0"},
	}).To_PhoneCallProtocol()
}

func secret() []byte {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	b[0] |= 0x40
	return b
}

type captured struct {
	notes []engine.Note
	ends  []engine.CallEnd
	timer []func()
}

func testEngine(t *testing.T) (*engine.Engine, *captured) {
	t.Helper()
	capn := &captured{}
	eng := engine.New(engine.Options{
		Config: config.Config{
			PublicIP: "203.0.113.10", PublicIPv6: "203.0.113.10",
			ReflectorPort: 3479, SFUPort: 3480, TurnHost: "203.0.113.10",
			TurnPort: 3478, StunPort: 3478, TurnSecret: "tellomi-turn-secret",
			TurnTTL: 3600, RingTimeout: 90, ConfirmTimeout: 30, EnableP2P: true,
			CallLinkPrefix: "https://t.me/call/", RTMPURL: "rtmp://127.0.0.1:1935/live",
		},
		Push:   func(n engine.Note) { capn.notes = append(capn.notes, n) },
		OnCall: func(e engine.CallEnd) { capn.ends = append(capn.ends, e) },
		Now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
		After: func(d time.Duration, fn func()) func() {
			capn.timer = append(capn.timer, fn)
			i := len(capn.timer) - 1
			return func() { capn.timer[i] = nil }
		},
	})
	return eng, capn
}

func roundTrip(t *testing.T, obj interface {
	Encode(*mtproto.EncodeBuf, int32) error
}, ctor uint32) []byte {
	t.Helper()
	buf := mtproto.NewEncodeBuf(4096)
	if err := obj.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	raw := append([]byte(nil), buf.GetBuf()...)
	if len(raw) < 4 {
		t.Fatal("short encode")
	}
	got := binary.LittleEndian.Uint32(raw[:4])
	if got != ctor {
		t.Fatalf("constructor 0x%x, want 0x%x", got, ctor)
	}
	return raw
}

func TestOneToOneCallAndLayer232(t *testing.T) {
	eng, capn := testEngine(t)
	svc := NewWithEngine(eng)
	a := secret()
	b := secret()
	ga := dh.Public(a)
	gb := dh.Public(b)
	sum := sha256.Sum256(ga)
	fp := dh.Fingerprint(dh.Shared(gb, a))
	if fp != dh.Fingerprint(dh.Shared(ga, b)) {
		t.Fatal("fingerprint mismatch")
	}

	ctxA := withUser(1, 11)
	ctxB := withUser(2, 22)
	res, err := svc.PhoneRequestCall(ctxA, &mtproto.TLPhoneRequestCall{
		Video: true, RandomId: 7,
		UserId:   mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2, AccessHash: 9}).To_InputUser(),
		GAHash:   sum[:],
		Protocol: callProto(),
	})
	if err != nil {
		t.Fatal(err)
	}
	call := res.GetPhoneCall()
	if call.GetPredicateName() != mtproto.Predicate_phoneCallWaiting || !call.GetVideo() {
		t.Fatalf("caller view %s", call.GetPredicateName())
	}
	if len(capn.notes) != 1 || capn.notes[0].UserID != 2 {
		t.Fatalf("ring notes %+v", capn.notes)
	}
	req := capn.notes[0].Updates.Updates[0].GetPhoneCall()
	if req.GetPredicateName() != mtproto.Predicate_phoneCallRequested || !dh.HashMatches(ga, req.GetGAHash()) {
		t.Fatal("requested update")
	}
	roundTrip(t, req, 0x14b0ed0c)
	roundTrip(t, call, 0xc5226f17)

	peer := mtproto.MakeTLInputPhoneCall(&mtproto.InputPhoneCall{Id: call.GetId(), AccessHash: call.GetAccessHash()}).To_InputPhoneCall()
	if _, err := svc.PhoneReceivedCall(ctxB, &mtproto.TLPhoneReceivedCall{Peer: peer}); err != nil {
		t.Fatal(err)
	}
	acc, err := svc.PhoneAcceptCall(ctxB, &mtproto.TLPhoneAcceptCall{Peer: peer, GB: gb, Protocol: callProto()})
	if err != nil {
		t.Fatal(err)
	}
	if acc.GetPhoneCall().GetPredicateName() != mtproto.Predicate_phoneCallWaiting {
		t.Fatal(acc.GetPhoneCall().GetPredicateName())
	}
	var accepted *mtproto.PhoneCall
	for _, n := range capn.notes {
		if n.UserID == 1 && n.Updates.Updates[0].GetPhoneCall().GetPredicateName() == mtproto.Predicate_phoneCallAccepted {
			accepted = n.Updates.Updates[0].GetPhoneCall()
		}
	}
	if accepted == nil || !dh.ValidPublic(accepted.GetGB()) {
		t.Fatal("accepted update")
	}
	roundTrip(t, accepted, 0x3660c311)

	bad := append([]byte(nil), ga...)
	bad[255] ^= 0xff
	if _, err := svc.PhoneConfirmCall(ctxA, &mtproto.TLPhoneConfirmCall{Peer: peer, GA: bad, KeyFingerprint: fp, Protocol: callProto()}); err == nil {
		t.Fatal("bad g_a accepted")
	}
	conf, err := svc.PhoneConfirmCall(ctxA, &mtproto.TLPhoneConfirmCall{Peer: peer, GA: ga, KeyFingerprint: fp, Protocol: callProto()})
	if err != nil {
		t.Fatal(err)
	}
	active := conf.GetPhoneCall()
	if active.GetPredicateName() != mtproto.Predicate_phoneCall || active.GetKeyFingerprint() != fp || !active.GetP2PAllowed() {
		t.Fatalf("active %+v", active)
	}
	if string(active.GetGAOrB()) != string(gb) {
		t.Fatal("caller g_a_or_b should be g_b")
	}
	var sawReflector, sawTurn, sawStun bool
	for _, c := range active.GetConnections() {
		switch c.GetPredicateName() {
		case mtproto.Predicate_phoneConnection:
			sawReflector = len(c.GetPeerTag()) == 16 && c.GetIp() == "203.0.113.10" && c.GetPort() == 3479
		case mtproto.Predicate_phoneConnectionWebrtc:
			if c.GetTurn() {
				sawTurn = c.GetUsername() != "" && c.GetPassword() != "" && c.GetPort() == 3478
			}
			if c.GetStun() {
				sawStun = true
			}
		}
	}
	if !sawReflector || !sawTurn || !sawStun {
		t.Fatalf("connections %+v", active.GetConnections())
	}
	roundTrip(t, active, 0x30535af5)
	for _, c := range active.GetConnections() {
		ctor := uint32(0x635fe375)
		if c.GetPredicateName() == mtproto.Predicate_phoneConnection {
			ctor = 0x9cc123c7
		}
		roundTrip(t, c, ctor)
	}

	var callee *mtproto.PhoneCall
	for _, n := range capn.notes {
		if n.UserID == 2 && n.Updates.Updates[0].GetPhoneCall().GetPredicateName() == mtproto.Predicate_phoneCall {
			callee = n.Updates.Updates[0].GetPhoneCall()
		}
	}
	if callee == nil || string(callee.GetGAOrB()) != string(ga) || !dh.HashMatches(callee.GetGAOrB(), sum[:]) {
		t.Fatal("callee did not receive g_a")
	}

	if _, err := svc.PhoneSendSignalingData(ctxA, &mtproto.TLPhoneSendSignalingData{Peer: peer, Data: []byte{9, 8, 7}}); err != nil {
		t.Fatal(err)
	}
	last := capn.notes[len(capn.notes)-1]
	if last.UserID != 2 || string(last.Updates.Updates[0].GetData_FLAGBYTES()) != string([]byte{9, 8, 7}) {
		t.Fatal("signaling")
	}
	roundTrip(t, last.Updates.Updates[0], 0x2661bf09)

	up, err := svc.PhoneDiscardCall(ctxA, &mtproto.TLPhoneDiscardCall{
		Peer: peer, Duration: 5, Video: true,
		Reason: mtproto.MakeTLPhoneCallDiscardReasonHangup(nil).To_PhoneCallDiscardReason(),
	})
	if err != nil {
		t.Fatal(err)
	}
	disc := up.Updates[0].GetPhoneCall()
	if disc.GetPredicateName() != mtproto.Predicate_phoneCallDiscarded || disc.GetDuration().GetValue() != 5 || !disc.GetNeedRating() {
		t.Fatalf("discard %+v", disc)
	}
	roundTrip(t, disc, 0x50ca4de1)
	if len(capn.ends) != 1 || !capn.ends[0].Connected || capn.ends[0].Duration != 5 {
		t.Fatalf("history %+v", capn.ends)
	}
	if _, err := svc.PhoneSetCallRating(ctxB, &mtproto.TLPhoneSetCallRating{Peer: peer, Rating: 5, Comment: "ok"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PhoneSaveCallDebug(ctxB, &mtproto.TLPhoneSaveCallDebug{
		Peer: peer, Debug: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: `{"ok":true}`}).To_DataJSON(),
	}); err != nil {
		t.Fatal(err)
	}
	if eng.DebugLog(call.GetId()) != `{"ok":true}` {
		t.Fatal(eng.DebugLog(call.GetId()))
	}
}

func TestCallTimeoutAndBusy(t *testing.T) {
	eng, capn := testEngine(t)
	svc := NewWithEngine(eng)
	ga := dh.Public(secret())
	sum := sha256.Sum256(ga)
	ctx := withUser(1, 1)
	res, err := svc.PhoneRequestCall(ctx, &mtproto.TLPhoneRequestCall{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 2}).To_InputUser(),
		GAHash: sum[:], Protocol: callProto(), RandomId: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if capn.timer[0] == nil {
		t.Fatal("no ring timer")
	}
	capn.timer[0]()
	if len(capn.ends) != 1 || capn.ends[0].Reason.GetPredicateName() != mtproto.Predicate_phoneCallDiscardReasonMissed {
		t.Fatalf("timeout %+v", capn.ends)
	}
	peer := mtproto.MakeTLInputPhoneCall(&mtproto.InputPhoneCall{
		Id: res.GetPhoneCall().GetId(), AccessHash: res.GetPhoneCall().GetAccessHash(),
	}).To_InputPhoneCall()
	if _, err := svc.PhoneAcceptCall(withUser(2, 2), &mtproto.TLPhoneAcceptCall{Peer: peer, GB: dh.Public(secret()), Protocol: callProto()}); err == nil {
		t.Fatal("accepted after timeout")
	}

	// A fresh pair can call. Occupying user 3 then calling them is busy.
	sum2 := sha256.Sum256(dh.Public(secret()))
	if _, err := svc.PhoneRequestCall(withUser(3, 3), &mtproto.TLPhoneRequestCall{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 4}).To_InputUser(),
		GAHash: sum2[:], Protocol: callProto(), RandomId: 2,
	}); err != nil {
		t.Fatal(err)
	}
	busy, err := svc.PhoneRequestCall(withUser(5, 5), &mtproto.TLPhoneRequestCall{
		UserId: mtproto.MakeTLInputUser(&mtproto.InputUser{UserId: 3}).To_InputUser(),
		GAHash: sum2[:], Protocol: callProto(), RandomId: 3,
	})
	if err != nil {
		t.Fatal(err)
	}
	if busy.GetPhoneCall().GetPredicateName() != mtproto.Predicate_phoneCallDiscarded ||
		busy.GetPhoneCall().GetReason().GetPredicateName() != mtproto.Predicate_phoneCallDiscardReasonBusy {
		t.Fatalf("busy %+v", busy.GetPhoneCall())
	}
}

func TestGroupCallAndDhConfig(t *testing.T) {
	ch := channel.New(channel.NewMemoryStore())
	channel.Use(ch)
	created, err := ch.Create(withUser(10, 1), channel.CreateRequest{CreatorID: 10, Title: "Voice", Megagroup: true})
	if err != nil {
		t.Fatal(err)
	}
	chat := created.Chats[0]
	var binds []engine.Bind
	eng := engine.New(engine.Options{
		Config: config.Config{PublicIP: "203.0.113.10", SFUPort: 3480, TurnPort: 3478, StunPort: 3478, TurnSecret: "s", CallLinkPrefix: "https://t.me/call/", RTMPURL: "rtmp://127.0.0.1:1935/live"}.Normalized(),
		OnBind: func(b engine.Bind) { binds = append(binds, b) },
		Now:    func() time.Time { return time.Unix(1_700_000_000, 0) },
		Chats: func(viewer int64, peerType int32, peerID int64) []*mtproto.Chat {
			if c := channel.Shared().VisibleChat(withUser(viewer, 0), viewer, peerID); c != nil {
				return []*mtproto.Chat{c}
			}
			return nil
		},
		Members: func(peerType int32, peerID int64) []int64 {
			ids, _ := channel.Shared().MemberIDs(withUser(10, 0), peerID)
			return ids
		},
	})
	svc := NewWithEngine(eng)
	// NewWithEngine does not install the channel hook. Install the same one service.New uses.
	channel.LookupGroupCall = func(ctx context.Context, channelID int64) channel.GroupCallAttach {
		for i := len(binds) - 1; i >= 0; i-- {
			if binds[i].PeerID != channelID {
				continue
			}
			if !binds[i].Active {
				return channel.GroupCallAttach{}
			}
			return channel.GroupCallAttach{
				Call:     mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: binds[i].CallID, AccessHash: binds[i].Access}).To_InputGroupCall(),
				Active:   true,
				NotEmpty: binds[i].NotEmpty,
			}
		}
		return channel.GroupCallAttach{}
	}

	ctx := withUser(10, 100)
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{
		ChannelId: chat.Id, AccessHash: chat.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputPeer()
	up, err := svc.PhoneCreateGroupCall(ctx, &mtproto.TLPhoneCreateGroupCall{Peer: peer, RandomId: 4, Title: wrapperspb.String("standup")})
	if err != nil {
		t.Fatal(err)
	}
	gcall := up.Updates[0].GetCall_GROUPCALL()
	if gcall.GetPredicateName() != mtproto.Predicate_groupCall || !gcall.GetCreator() || gcall.GetTitle().GetValue() != "standup" {
		t.Fatalf("group %+v", gcall)
	}
	roundTrip(t, gcall, 0xefb2b617)
	full, err := ch.GetFullChannel(ctx, 10, inputChannel(chat))
	if err != nil || full.GetFullChat().GetCall().GetId() != gcall.GetId() {
		t.Fatalf("full call %+v %v", full.GetFullChat().GetCall(), err)
	}
	visible := ch.VisibleChat(ctx, 10, chat.Id)
	if visible == nil || !visible.GetCallActive() {
		t.Fatal("call_active")
	}

	inCall := mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: gcall.GetId(), AccessHash: gcall.GetAccessHash()}).To_InputGroupCall()
	params := `{"ssrc":222,"ufrag":"cu","pwd":"cp","fingerprints":[{"hash":"sha-256","fingerprint":"AA:BB","setup":"passive"}]}`
	joined, err := svc.PhoneJoinGroupCall(ctx, &mtproto.TLPhoneJoinGroupCall{
		Muted: true, Call: inCall, JoinAs: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Params: mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: params}).To_DataJSON(),
	})
	if err != nil {
		t.Fatal(err)
	}
	var connJSON string
	var participant *mtproto.GroupCallParticipant
	for _, u := range joined.Updates {
		if u.GetPredicateName() == mtproto.Predicate_updateGroupCallConnection {
			connJSON = u.GetParams().GetData()
			roundTrip(t, u, 0x0b783982)
		}
		if u.GetPredicateName() == mtproto.Predicate_updateGroupCallParticipants {
			participant = u.GetParticipants_VECTORGROUPCALLPARTICIPANT()[0]
			roundTrip(t, u, 0xf2ebdb4e)
		}
	}
	if connJSON == "" || participant == nil || participant.GetSource() != 222 || !participant.GetSelf() || !participant.GetMuted() {
		t.Fatalf("join %s %+v", connJSON, participant)
	}
	if _, err := svc.PhoneEditGroupCallParticipant(ctx, &mtproto.TLPhoneEditGroupCallParticipant{
		Call: inCall, Participant: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
		Muted: mtproto.BoolFalse, RaiseHand: mtproto.BoolTrue,
	}); err != nil {
		t.Fatal(err)
	}
	view, err := svc.PhoneGetGroupCall(ctx, &mtproto.TLPhoneGetGroupCall{Call: inCall, Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if view.GetParticipants()[0].GetMuted() || view.GetParticipants()[0].GetRaiseHandRating().GetValue() == 0 {
		t.Fatalf("participant %+v", view.GetParticipants()[0])
	}
	roundTrip(t, view, 0x9e727aad)
	link, err := svc.PhoneExportGroupCallInvite(ctx, &mtproto.TLPhoneExportGroupCallInvite{Call: inCall, CanSelfUnmute: true})
	if err != nil || link.GetLink() == "" {
		t.Fatal(link, err)
	}
	if _, err := svc.PhoneToggleGroupCallRecord(ctx, &mtproto.TLPhoneToggleGroupCallRecord{Call: inCall, Start: true, Video: true}); err != nil {
		t.Fatal(err)
	}
	again, _ := svc.PhoneGetGroupCall(ctx, &mtproto.TLPhoneGetGroupCall{Call: inCall, Limit: 1})
	if again.GetCall().GetRecordStartDate().GetValue() == 0 || !again.GetCall().GetRecordVideoActive() {
		t.Fatal("record flag")
	}
	rtmp, err := svc.PhoneGetGroupCallStreamRtmpUrl(ctx, &mtproto.TLPhoneGetGroupCallStreamRtmpUrl{
		Peer: peer, Revoke: mtproto.BoolFalse,
	})
	if err != nil || rtmp.GetUrl() == "" || rtmp.GetKey() == "" {
		t.Fatal(rtmp, err)
	}
	streams, err := svc.PhoneGetGroupCallStreamChannels(ctx, &mtproto.TLPhoneGetGroupCallStreamChannels{Call: inCall})
	if err != nil || len(streams.GetChannels()) != 1 {
		t.Fatal(streams, err)
	}
	sources, err := svc.PhoneCheckGroupCall(ctx, &mtproto.TLPhoneCheckGroupCall{Call: inCall, Sources: []int32{222, 1}})
	if err != nil || len(sources.GetDatas()) != 1 || sources.Datas[0] != 222 {
		t.Fatalf("%+v %v", sources, err)
	}
	if _, err := svc.PhoneLeaveGroupCall(ctx, &mtproto.TLPhoneLeaveGroupCall{Call: inCall, Source: 222}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.PhoneDiscardGroupCall(ctx, &mtproto.TLPhoneDiscardGroupCall{Call: inCall}); err != nil {
		t.Fatal(err)
	}
	full, _ = ch.GetFullChannel(ctx, 10, inputChannel(chat))
	if full.GetFullChat().GetCall() != nil {
		t.Fatal("call still attached")
	}

	cfg, err := svc.MessagesGetDhConfig(ctx, &mtproto.TLMessagesGetDhConfig{Version: 0, RandomLength: 16})
	if err != nil || cfg.GetG() != 3 || len(cfg.GetP()) != 256 || len(cfg.GetRandom()) != 16 {
		t.Fatalf("%+v %v", cfg, err)
	}
	roundTrip(t, cfg, 0x2c221edd)
	same, err := svc.MessagesGetDhConfig(ctx, &mtproto.TLMessagesGetDhConfig{Version: dh.Version, RandomLength: 8})
	if err != nil || same.GetPredicateName() != mtproto.Predicate_messages_dhConfigNotModified {
		t.Fatal(same, err)
	}
	if _, err := svc.MessagesGetDhConfig(ctx, &mtproto.TLMessagesGetDhConfig{RandomLength: 1000}); err == nil {
		t.Fatal("long random accepted")
	}
}

func inputChannel(ch *mtproto.Chat) *mtproto.InputChannel {
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId: ch.Id, AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
}
