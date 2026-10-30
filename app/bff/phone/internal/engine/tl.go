package engine

import (
	"strconv"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/relay"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/sfu"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func dataJSON(s string) *mtproto.DataJSON {
	return mtproto.MakeTLDataJSON(&mtproto.DataJSON{Data: s}).To_DataJSON()
}

func reasonMissed() *mtproto.PhoneCallDiscardReason {
	return mtproto.MakeTLPhoneCallDiscardReasonMissed(nil).To_PhoneCallDiscardReason()
}

func reasonDisconnect() *mtproto.PhoneCallDiscardReason {
	return mtproto.MakeTLPhoneCallDiscardReasonDisconnect(nil).To_PhoneCallDiscardReason()
}

func reasonHangup() *mtproto.PhoneCallDiscardReason {
	return mtproto.MakeTLPhoneCallDiscardReasonHangup(nil).To_PhoneCallDiscardReason()
}

func reasonBusy() *mtproto.PhoneCallDiscardReason {
	return mtproto.MakeTLPhoneCallDiscardReasonBusy(nil).To_PhoneCallDiscardReason()
}

func cloneProtocol(p *mtproto.PhoneCallProtocol) *mtproto.PhoneCallProtocol {
	if p == nil {
		return nil
	}
	cp := *p
	cp.LibraryVersions = append([]string(nil), p.LibraryVersions...)
	return mtproto.MakeTLPhoneCallProtocol(&cp).To_PhoneCallProtocol()
}

func intersectProtocol(a, b *mtproto.PhoneCallProtocol) (*mtproto.PhoneCallProtocol, error) {
	if a == nil || b == nil {
		return nil, mtproto.ErrCallProtocolFlagsInvalid
	}
	minL, maxL := a.MinLayer, a.MaxLayer
	if b.MinLayer > minL {
		minL = b.MinLayer
	}
	if b.MaxLayer < maxL {
		maxL = b.MaxLayer
	}
	if minL > maxL || maxL <= 0 {
		return nil, mtproto.ErrCallProtocolCompatLayerInvalid
	}
	have := map[string]bool{}
	for _, v := range b.LibraryVersions {
		have[v] = true
	}
	var common []string
	for _, v := range a.LibraryVersions {
		if have[v] {
			common = append(common, v)
		}
	}
	if len(common) == 0 {
		return nil, mtproto.ErrParticipantVersionOutdated
	}
	p2p := a.UdpP2P && b.UdpP2P
	refl := a.UdpReflector && b.UdpReflector
	if !p2p && !refl {
		return nil, mtproto.ErrCallProtocolFlagsInvalid
	}
	return mtproto.MakeTLPhoneCallProtocol(&mtproto.PhoneCallProtocol{
		UdpP2P:          p2p,
		UdpReflector:    refl,
		MinLayer:        minL,
		MaxLayer:        maxL,
		LibraryVersions: common,
	}).To_PhoneCallProtocol(), nil
}

func peerOf(peerType int32, id int64) *mtproto.Peer {
	switch peerType {
	case mtproto.PEER_CHAT:
		return mtproto.MakePeerChat(id)
	case mtproto.PEER_CHANNEL:
		return mtproto.MakePeerChannel(id)
	default:
		return mtproto.MakePeerUser(id)
	}
}

func inputCall(id, access int64) *mtproto.InputGroupCall {
	return mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: id, AccessHash: access}).To_InputGroupCall()
}

func (e *Engine) connections(userID int64, tag []byte) []*mtproto.PhoneConnection {
	cfg := e.cfg
	now := e.now()
	user := relay.TURNUsername(strconv.FormatInt(userID, 10), time.Duration(cfg.TurnTTL)*time.Second, now)
	pass := relay.TURNPassword(cfg.TurnSecret, user)
	ipv6 := cfg.IPv6()
	var out []*mtproto.PhoneConnection
	port := cfg.ReflectorPort
	if e.relay != nil && e.relay.Addr() != nil {
		port = e.relay.Addr().Port
	}
	if port > 0 && len(tag) == 16 {
		out = append(out, mtproto.MakeTLPhoneConnection(&mtproto.PhoneConnection{
			Id: newID(), Ip: cfg.PublicIP, Ipv6: ipv6, Port: int32(port), PeerTag: append([]byte(nil), tag...),
		}).To_PhoneConnection())
	}
	for _, ep := range cfg.TURNEndpoints() {
		out = append(out, mtproto.MakeTLPhoneConnectionWebrtc(&mtproto.PhoneConnection{
			Turn: true, Id: newID(), Ip: ep.Host, Ipv6: ipv6, Port: int32(ep.Port),
			Username: user, Password: pass,
		}).To_PhoneConnection())
	}
	for _, ep := range cfg.STUNEndpoints() {
		out = append(out, mtproto.MakeTLPhoneConnectionWebrtc(&mtproto.PhoneConnection{
			Stun: true, Id: newID(), Ip: ep.Host, Ipv6: ipv6, Port: int32(ep.Port),
			Username: user, Password: pass,
		}).To_PhoneConnection())
	}
	return out
}

func (e *Engine) phoneView(c *call, forAdmin bool) *mtproto.PhoneCall {
	proto := c.callerProto
	if c.agreed != nil {
		proto = c.agreed
	}
	base := &mtproto.PhoneCall{
		Video:         c.video,
		Id:            c.id,
		AccessHash:    c.access,
		Date:          c.date,
		AdminId:       c.admin,
		ParticipantId: c.participant,
		Protocol:      proto,
	}
	switch c.phase {
	case phaseAccepted:
		if forAdmin {
			base.GB = append([]byte(nil), c.gb...)
			return mtproto.MakeTLPhoneCallAccepted(base).To_PhoneCall()
		}
		if c.receive != 0 {
			base.ReceiveDate = wrapperspb.Int32(c.receive)
		}
		return mtproto.MakeTLPhoneCallWaiting(base).To_PhoneCall()
	case phaseActive:
		other := c.ga
		tag := c.partTag
		uid := c.participant
		if forAdmin {
			other = c.gb
			tag = c.adminTag
			uid = c.admin
		}
		base.P2PAllowed = c.p2p
		base.GAOrB = append([]byte(nil), other...)
		base.KeyFingerprint = c.fingerprint
		base.Connections = e.connections(uid, tag)
		base.StartDate = c.start
		base.CustomParameters = dataJSON(config.CustomParamsJSON)
		return mtproto.MakeTLPhoneCall(base).To_PhoneCall()
	case phaseDiscarded:
		d := &mtproto.PhoneCall{
			Video:      c.video,
			Id:         c.id,
			NeedRating: c.needRating,
			NeedDebug:  c.needDebug,
			Reason:     c.reason,
		}
		if c.wasActive && c.duration >= 0 {
			d.Duration = wrapperspb.Int32(c.duration)
		}
		return mtproto.MakeTLPhoneCallDiscarded(d).To_PhoneCall()
	default:
		if !forAdmin {
			base.GAHash = append([]byte(nil), c.gaHash...)
			return mtproto.MakeTLPhoneCallRequested(base).To_PhoneCall()
		}
		if c.receive != 0 {
			base.ReceiveDate = wrapperspb.Int32(c.receive)
		}
		return mtproto.MakeTLPhoneCallWaiting(base).To_PhoneCall()
	}
}

func (e *Engine) phoneResult(viewer int64, c *call, forAdmin bool) *mtproto.Phone_PhoneCall {
	users := []*mtproto.User{}
	if e.users != nil {
		if got := e.users(viewer, []int64{c.admin, c.participant}); got != nil {
			users = got
		}
	}
	return mtproto.MakeTLPhonePhoneCall(&mtproto.Phone_PhoneCall{
		PhoneCall: e.phoneView(c, forAdmin),
		Users:     users,
	}).To_Phone_PhoneCall()
}

func phoneUpdate(call *mtproto.PhoneCall) *mtproto.Update {
	return mtproto.MakeTLUpdatePhoneCall(&mtproto.Update{PhoneCall: call}).To_Update()
}

func (e *Engine) notesFor(c *call, actor int64, users ...int64) []Note {
	var notes []Note
	for _, uid := range users {
		admin := uid == c.admin
		ids := []int64{c.admin, c.participant}
		up := e.ups(uid, ids, phoneUpdate(e.phoneView(c, admin)))
		var exclude []int64
		if uid == actor {
			switch uid {
			case c.admin:
				if c.adminAuth != 0 {
					exclude = []int64{c.adminAuth}
				}
			case c.participant:
				if c.partAuth != 0 {
					exclude = []int64{c.partAuth}
				}
			}
		}
		notes = append(notes, Note{UserID: uid, Exclude: exclude, Updates: up})
	}
	return notes
}

func (e *Engine) ups(viewer int64, ids []int64, updates ...*mtproto.Update) *mtproto.Updates {
	var users []*mtproto.User
	if e.users != nil && len(ids) > 0 {
		users = e.users(viewer, ids)
	}
	return mtproto.MakeUpdatesByUpdatesUsers(users, updates...)
}

func (e *Engine) answerOffer(callID int64, key, raw, endpoint string) (string, uint32, []sfu.Group, error) {
	if e.media != nil {
		answer, ssrc, video, err := e.media.Join(callID, key, raw, endpoint)
		if err != nil {
			return "", 0, nil, err
		}
		offer, _ := sfu.ParseOffer(raw)
		groups := []sfu.Group{}
		if video {
			groups = offer.SourceGroups()
		}
		return answer, ssrc, groups, nil
	}
	offer, err := sfu.ParseOffer(raw)
	if err != nil {
		return "", 0, nil, err
	}
	port := e.cfg.SFUPort
	if port == 0 {
		port = 3480
	}
	answer, err := sfu.AnswerJSON(offer, sfu.Local{
		Ufrag: iceFallback(4), Pwd: iceFallback(16), Fingerprint: "00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF:00:11:22:33:44:55:66:77:88:99:AA:BB:CC:DD:EE:FF",
		IP: e.cfg.PublicIP, Port: port, Endpoint: endpoint,
	})
	if err != nil {
		return "", 0, nil, err
	}
	return answer, offer.AudioSSRC, offer.SourceGroups(), nil
}

func iceFallback(n int) string {
	s := "tellomicecredential!!"
	if n > len(s) {
		n = len(s)
	}
	return s[:n]
}
