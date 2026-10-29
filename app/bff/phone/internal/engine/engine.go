// Package engine is the phone call and group-call state machine.
package engine

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"sync"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/config"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/dh"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/relay"
	"github.com/teamgram/teamgram-server/app/bff/phone/internal/sfu"
)

type phase int

const (
	phaseWaiting phase = iota
	phaseAccepted
	phaseActive
	phaseDiscarded
)

// Note is one update fan-out. Exclude lists auth keys that already got the
// same updates in the RPC response.
type Note struct {
	UserID  int64
	Exclude []int64
	Updates *mtproto.Updates
}

// CallEnd is a finished 1:1 call, used to write messageActionPhoneCall.
type CallEnd struct {
	Admin, Participant int64
	Video              bool
	CallID             int64
	Reason             *mtproto.PhoneCallDiscardReason
	Duration           int32
	Connected          bool
}

// GroupEnd is a group-call service message (started, discarded, invite).
type GroupEnd struct {
	From     int64
	PeerType int32
	PeerID   int64
	Action   *mtproto.MessageAction
	RandomID int64
}

// Bind is the chat/channel → group call row.
type Bind struct {
	PeerType         int32
	PeerID           int64
	CallID, Access   int64
	Active, NotEmpty bool
}

// Engine holds live calls. Push, history, and privacy hooks are optional.
type Engine struct {
	cfg     config.Config
	relay   *relay.Reflector
	media   *sfu.Server
	push    func(Note)
	onCall  func(CallEnd)
	onGroup func(GroupEnd)
	onBind  func(Bind)
	allow   func(caller, callee int64) (call bool, p2p bool)
	users   func(viewer int64, ids []int64) []*mtproto.User
	chats   func(viewer int64, peerType int32, peerID int64) []*mtproto.Chat
	members func(peerType int32, peerID int64) []int64
	now     func() time.Time
	after   func(time.Duration, func()) func()

	mu       sync.Mutex
	calls    map[int64]*call
	byUser   map[int64]int64
	byRandom map[string]int64
	groups   map[int64]*group
	byPeer   map[string]int64
	rtmpKeys map[string]string
}

type call struct {
	id, access           int64
	admin, participant   int64
	adminAuth, partAuth  int64
	video, p2p           bool
	date, receive, start int32
	gaHash, ga, gb       []byte
	fingerprint          int64
	callerProto          *mtproto.PhoneCallProtocol
	agreed               *mtproto.PhoneCallProtocol
	phase                phase
	reason               *mtproto.PhoneCallDiscardReason
	duration             int32
	wasActive            bool
	needRating           bool
	needDebug            bool
	rating               int32
	comment, debug       string
	randomID             int32
	adminTag, partTag    []byte
	cancel               func()
}

// Options builds an Engine. Nil hooks are skipped.
type Options struct {
	Config  config.Config
	Relay   *relay.Reflector
	Media   *sfu.Server
	Push    func(Note)
	OnCall  func(CallEnd)
	OnGroup func(GroupEnd)
	OnBind  func(Bind)
	Allow   func(caller, callee int64) (call bool, p2p bool)
	Users   func(viewer int64, ids []int64) []*mtproto.User
	Chats   func(viewer int64, peerType int32, peerID int64) []*mtproto.Chat
	Members func(peerType int32, peerID int64) []int64
	Now     func() time.Time
	After   func(time.Duration, func()) func()
}

func New(o Options) *Engine {
	cfg := o.Config.Normalized()
	e := &Engine{
		cfg: cfg, relay: o.Relay, media: o.Media,
		push: o.Push, onCall: o.OnCall, onGroup: o.OnGroup, onBind: o.OnBind,
		allow: o.Allow, users: o.Users, chats: o.Chats, members: o.Members,
		now: o.Now, after: o.After,
		calls: map[int64]*call{}, byUser: map[int64]int64{}, byRandom: map[string]int64{},
		groups: map[int64]*group{}, byPeer: map[string]int64{}, rtmpKeys: map[string]string{},
	}
	if e.now == nil {
		e.now = time.Now
	}
	if e.after == nil {
		e.after = func(d time.Duration, fn func()) func() {
			t := time.AfterFunc(d, fn)
			return func() { t.Stop() }
		}
	}
	return e
}

func newID() int64 {
	var b [8]byte
	_, _ = rand.Read(b[:])
	v := int64(binary.BigEndian.Uint64(b[:]) & 0x7fffffffffffffff)
	if v == 0 {
		return 1
	}
	return v
}

func peerTag() []byte {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return b
}

func (e *Engine) deliver(notes []Note) {
	if e.push == nil {
		return
	}
	for _, n := range notes {
		if n.Updates != nil {
			e.push(n)
		}
	}
}

func (e *Engine) finishCall(end *CallEnd) {
	if end != nil && e.onCall != nil {
		e.onCall(*end)
	}
}

func (e *Engine) finishGroup(ev *GroupEnd, bind *Bind) {
	if ev != nil && e.onGroup != nil {
		e.onGroup(*ev)
	}
	if bind != nil && e.onBind != nil {
		e.onBind(*bind)
	}
}

// CallConfig is phone.getCallConfig.
func (e *Engine) CallConfig() *mtproto.DataJSON {
	return dataJSON(config.CallConfigJSON)
}

// Request is phone.requestCall after the handler has resolved the callee.
type Request struct {
	Admin, AdminAuth int64
	Participant      int64
	Video            bool
	RandomID         int32
	GAHash           []byte
	Protocol         *mtproto.PhoneCallProtocol
}

func (e *Engine) RequestCall(in Request) (*mtproto.Phone_PhoneCall, error) {
	if in.Admin == 0 || in.Participant == 0 || in.Admin == in.Participant {
		return nil, mtproto.ErrCallPeerInvalid
	}
	if len(in.GAHash) != 32 {
		return nil, mtproto.ErrDhGAInvalid
	}
	if in.Protocol == nil || (!in.Protocol.UdpP2P && !in.Protocol.UdpReflector) {
		return nil, mtproto.ErrCallProtocolFlagsInvalid
	}
	p2p := e.cfg.EnableP2P && in.Protocol.UdpP2P
	if e.allow != nil {
		ok, p2pOK := e.allow(in.Admin, in.Participant)
		if !ok {
			return nil, mtproto.ErrUserPrivacyRestricted
		}
		p2p = p2p && p2pOK
	}
	e.mu.Lock()
	key := fmt.Sprintf("%d:%d", in.Admin, in.RandomID)
	if id, ok := e.byRandom[key]; ok {
		if c := e.calls[id]; c != nil && c.phase != phaseDiscarded {
			res := e.phoneResult(in.Admin, c, true)
			e.mu.Unlock()
			return res, nil
		}
	}
	if e.busy(in.Admin) || e.busy(in.Participant) {
		c := e.busyCall(in, p2p)
		res := e.phoneResult(in.Admin, c, true)
		end := e.endOf(c)
		e.mu.Unlock()
		e.finishCall(end)
		return res, nil
	}
	c := &call{
		id: newID(), access: newID(), admin: in.Admin, participant: in.Participant,
		adminAuth: in.AdminAuth, video: in.Video, p2p: p2p,
		date: int32(e.now().Unix()), gaHash: append([]byte(nil), in.GAHash...),
		callerProto: cloneProtocol(in.Protocol), phase: phaseWaiting, randomID: in.RandomID,
		adminTag: peerTag(), partTag: peerTag(),
	}
	e.calls[c.id] = c
	e.byUser[in.Admin] = c.id
	e.byUser[in.Participant] = c.id
	e.byRandom[key] = c.id
	e.arm(c, e.cfg.Ring(), "ring")
	res := e.phoneResult(in.Admin, c, true)
	notes := e.notesFor(c, in.Admin, in.Participant)
	e.mu.Unlock()
	e.deliver(notes)
	return res, nil
}

func (e *Engine) busy(user int64) bool {
	id, ok := e.byUser[user]
	if !ok {
		return false
	}
	c := e.calls[id]
	return c != nil && c.phase != phaseDiscarded
}

func (e *Engine) busyCall(in Request, p2p bool) *call {
	c := &call{
		id: newID(), access: newID(), admin: in.Admin, participant: in.Participant,
		video: in.Video, p2p: p2p, date: int32(e.now().Unix()),
		callerProto: cloneProtocol(in.Protocol), phase: phaseDiscarded,
		reason: reasonBusy(), randomID: in.RandomID,
	}
	e.calls[c.id] = c
	return c
}

func (e *Engine) arm(c *call, d time.Duration, kind string) {
	if c.cancel != nil {
		c.cancel()
	}
	id := c.id
	c.cancel = e.after(d, func() { e.expire(id, kind) })
}

func (e *Engine) expire(id int64, kind string) {
	e.mu.Lock()
	c := e.calls[id]
	if c == nil || c.phase == phaseDiscarded {
		e.mu.Unlock()
		return
	}
	if kind == "ring" && c.phase != phaseWaiting {
		e.mu.Unlock()
		return
	}
	if kind == "confirm" && c.phase != phaseAccepted {
		e.mu.Unlock()
		return
	}
	reason := reasonMissed()
	if kind == "confirm" {
		reason = reasonDisconnect()
	}
	notes, end := e.discardLocked(c, 0, reason, 0)
	e.mu.Unlock()
	e.deliver(notes)
	e.finishCall(end)
}

func (e *Engine) load(id, access int64) (*call, error) {
	c := e.calls[id]
	if c == nil || c.access != access {
		return nil, mtproto.ErrCallPeerInvalid
	}
	return c, nil
}

func (e *Engine) Received(user, auth, id, access int64) (*mtproto.Bool, error) {
	e.mu.Lock()
	c, err := e.load(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if user != c.participant {
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	}
	if c.phase == phaseDiscarded {
		e.mu.Unlock()
		return nil, mtproto.ErrCallAlreadyDeclined
	}
	if c.receive == 0 {
		c.receive = int32(e.now().Unix())
	}
	c.partAuth = auth
	notes := e.notesFor(c, user, c.admin)
	e.mu.Unlock()
	e.deliver(notes)
	return mtproto.BoolTrue, nil
}

func (e *Engine) Accept(user, auth, id, access int64, gb []byte, proto *mtproto.PhoneCallProtocol) (*mtproto.Phone_PhoneCall, error) {
	if !dh.ValidPublic(gb) {
		return nil, mtproto.ErrDhGAInvalid
	}
	e.mu.Lock()
	c, err := e.load(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if user != c.participant {
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	}
	switch c.phase {
	case phaseDiscarded:
		e.mu.Unlock()
		return nil, mtproto.ErrCallAlreadyDeclined
	case phaseAccepted, phaseActive:
		e.mu.Unlock()
		return nil, mtproto.ErrCallAlreadyAccepted
	}
	agreed, err := intersectProtocol(c.callerProto, proto)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	c.gb = append([]byte(nil), gb...)
	c.agreed = agreed
	c.phase = phaseAccepted
	c.partAuth = auth
	if c.receive == 0 {
		c.receive = int32(e.now().Unix())
	}
	e.arm(c, e.cfg.Confirm(), "confirm")
	res := e.phoneResult(user, c, false)
	notes := e.notesFor(c, user, c.admin)
	e.mu.Unlock()
	e.deliver(notes)
	return res, nil
}

func (e *Engine) Confirm(user, auth, id, access int64, ga []byte, fingerprint int64, proto *mtproto.PhoneCallProtocol) (*mtproto.Phone_PhoneCall, error) {
	if len(ga) == 0 {
		return nil, mtproto.ErrDhGAInvalid
	}
	e.mu.Lock()
	c, err := e.load(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if user != c.admin {
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	}
	switch c.phase {
	case phaseDiscarded:
		e.mu.Unlock()
		return nil, mtproto.ErrCallAlreadyDeclined
	case phaseWaiting:
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	case phaseActive:
		res := e.phoneResult(user, c, true)
		e.mu.Unlock()
		return res, nil
	}
	if !dh.HashMatches(ga, c.gaHash) || !dh.ValidPublic(ga) {
		e.mu.Unlock()
		return nil, mtproto.ErrDhGAInvalid
	}
	if proto != nil {
		if agreed, err := intersectProtocol(c.callerProto, proto); err == nil {
			c.agreed = agreed
		}
	}
	c.ga = append([]byte(nil), ga...)
	c.fingerprint = fingerprint
	c.phase = phaseActive
	c.wasActive = true
	c.start = int32(e.now().Unix())
	c.adminAuth = auth
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	if e.relay != nil {
		e.relay.Pair(c.adminTag, c.partTag)
	}
	res := e.phoneResult(user, c, true)
	notes := e.notesFor(c, user, c.admin, c.participant)
	e.mu.Unlock()
	e.deliver(notes)
	return res, nil
}

func (e *Engine) Discard(user, id, access int64, duration int32, reason *mtproto.PhoneCallDiscardReason) (*mtproto.Updates, error) {
	e.mu.Lock()
	c, err := e.load(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if user != c.admin && user != c.participant {
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	}
	if c.phase == phaseDiscarded {
		e.mu.Unlock()
		return nil, mtproto.ErrCallAlreadyDeclined
	}
	if reason == nil {
		if c.phase == phaseActive {
			reason = reasonHangup()
		} else if user == c.admin {
			reason = reasonMissed()
		} else {
			reason = reasonHangup()
		}
	}
	notes, end := e.discardLocked(c, user, reason, duration)
	up := e.ups(user, []int64{c.admin, c.participant}, phoneUpdate(e.phoneView(c, user == c.admin)))
	e.mu.Unlock()
	e.deliver(notes)
	e.finishCall(end)
	return up, nil
}

func (e *Engine) discardLocked(c *call, actor int64, reason *mtproto.PhoneCallDiscardReason, duration int32) ([]Note, *CallEnd) {
	if c.cancel != nil {
		c.cancel()
		c.cancel = nil
	}
	was := c.phase == phaseActive || c.wasActive
	if was {
		spent := int32(e.now().Unix()) - c.start
		if spent < 0 {
			spent = 0
		}
		if duration < spent {
			duration = spent
		}
		c.needRating = true
		c.needDebug = true
		c.wasActive = true
	} else {
		duration = 0
	}
	c.phase = phaseDiscarded
	c.reason = reason
	c.duration = duration
	delete(e.byUser, c.admin)
	delete(e.byUser, c.participant)
	if e.relay != nil {
		e.relay.Unpair(c.adminTag, c.partTag)
	}
	return e.notesFor(c, actor, c.admin, c.participant), e.endOf(c)
}

func (e *Engine) endOf(c *call) *CallEnd {
	dur := c.duration
	if !c.wasActive {
		dur = 0
	}
	return &CallEnd{
		Admin: c.admin, Participant: c.participant, Video: c.video, CallID: c.id,
		Reason: c.reason, Duration: dur, Connected: c.wasActive,
	}
}

func (e *Engine) Signaling(user, id, access int64, data []byte) (*mtproto.Bool, error) {
	if len(data) == 0 {
		return nil, mtproto.ErrCallPeerInvalid
	}
	e.mu.Lock()
	c, err := e.load(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if user != c.admin && user != c.participant {
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	}
	if c.phase == phaseDiscarded {
		e.mu.Unlock()
		return nil, mtproto.ErrCallAlreadyDeclined
	}
	if c.phase != phaseActive && c.phase != phaseAccepted {
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	}
	other := c.participant
	if user == c.participant {
		other = c.admin
	}
	up := mtproto.MakeUpdatesByUpdates(mtproto.MakeTLUpdatePhoneCallSignalingData(&mtproto.Update{
		PhoneCallId:    c.id,
		Data_FLAGBYTES: append([]byte(nil), data...),
	}).To_Update())
	e.mu.Unlock()
	e.deliver([]Note{{UserID: other, Updates: up}})
	return mtproto.BoolTrue, nil
}

func (e *Engine) Rate(user, id, access int64, rating int32, comment string) (*mtproto.Updates, error) {
	e.mu.Lock()
	c, err := e.load(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if user != c.admin && user != c.participant {
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	}
	c.rating = rating
	c.comment = comment
	e.mu.Unlock()
	return mtproto.MakeEmptyUpdates(), nil
}

func (e *Engine) Debug(user, id, access int64, debug string) (*mtproto.Bool, error) {
	e.mu.Lock()
	c, err := e.load(id, access)
	if err != nil {
		e.mu.Unlock()
		return nil, err
	}
	if user != c.admin && user != c.participant {
		e.mu.Unlock()
		return nil, mtproto.ErrCallPeerInvalid
	}
	c.debug = debug
	e.mu.Unlock()
	return mtproto.BoolTrue, nil
}

// DebugLog returns the stored debug JSON. Tests use it.
func (e *Engine) DebugLog(id int64) string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if c := e.calls[id]; c != nil {
		return c.debug
	}
	return ""
}
