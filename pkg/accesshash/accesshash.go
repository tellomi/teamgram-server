// Package accesshash checks the access_hash on inputUser, inputPeerUser,
// inputChannel, and inputPeerChannel.
//
// Teamgram stores one random int64 on the user row and one on the channel
// row (users.access_hash, channels.access_hash) and returns that same value
// to every viewer. Official Telegram derives a per-viewer hash. Replacing
// the stored value would invalidate the hashes Android and iOS already
// cached, so the check compares against the value this server already
// issued.
//
// TELLOMI_ACCESS_HASH_ENFORCE selects the mode:
//
//	unset, 0, false, off — do nothing (the default)
//	log                  — record mismatches, still accept the RPC
//	1, true, on, enforce — reject a mismatch
//
// inputUserSelf and inputPeerSelf are not checked. inputUserFromMessage,
// inputPeerUserFromMessage, inputChannelFromMessage, and
// inputPeerChannelFromMessage are accepted only when that message is visible
// to the viewer and actually refers to the user or channel.
//
// A stored access_hash of 0 matches only a provided 0. rand.Int63 and the
// seeded official rows are non-zero, so a guessed 0 does not open those
// accounts. Chatbox bots whose stored hash is 0 are issued the bot id
// (see botProfile.accessHash); that issued value still matches. Reaction
// lists and poll voters are still returned with access_hash omitted. Log
// mode is how to see clients echo that 0 before enforcement is turned on.
package accesshash

import (
	"context"
	"crypto/subtle"
	"encoding/binary"
	"os"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/proto"
)

// EnvEnforce is the process environment variable that selects the mode.
const EnvEnforce = "TELLOMI_ACCESS_HASH_ENFORCE"

// Mode is how a mismatch is treated.
type Mode int

const (
	// ModeOff accepts every hash. This is the default.
	ModeOff Mode = iota
	// ModeLog records mismatches and still accepts the RPC.
	ModeLog
	// ModeEnforce rejects a mismatch with the official RPC error.
	ModeEnforce
)

// Kind is which input constructor failed.
type Kind int

const (
	KindNone Kind = iota
	KindUser
	KindPeerUser
	KindChannel
	KindPeerChannel
	KindUserFromMessage
	KindPeerUserFromMessage
	KindChannelFromMessage
	KindPeerChannelFromMessage
	KindSelf
	KindChat
)

// Subject is one input constructor found on an RPC request.
type Subject struct {
	Kind     Kind
	ID       int64
	Provided int64
	MsgID    int32
	Peer     *mtproto.InputPeer
}

func (k Kind) String() string {
	switch k {
	case KindUser:
		return "inputUser"
	case KindPeerUser:
		return "inputPeerUser"
	case KindChannel:
		return "inputChannel"
	case KindPeerChannel:
		return "inputPeerChannel"
	case KindUserFromMessage:
		return "inputUserFromMessage"
	case KindPeerUserFromMessage:
		return "inputPeerUserFromMessage"
	case KindChannelFromMessage:
		return "inputChannelFromMessage"
	case KindPeerChannelFromMessage:
		return "inputPeerChannelFromMessage"
	case KindSelf:
		return "self"
	case KindChat:
		return "chat"
	default:
		return "none"
	}
}

// Error is the RPC error official clients expect for this constructor.
func (k Kind) Error() error {
	switch k {
	case KindUser, KindUserFromMessage:
		return mtproto.ErrUserIdInvalid
	case KindChannel, KindChannelFromMessage:
		return mtproto.ErrChannelInvalid
	default:
		return mtproto.ErrPeerIdInvalid
	}
}

// Book loads the hashes this process already issued and the messages a
// from-message constructor points at.
type Book interface {
	// UserHashes returns every access_hash the server may already have
	// given out for each id. A missing id has an empty slice.
	UserHashes(ctx context.Context, viewer int64, ids []int64) (map[int64][]int64, error)
	// ChannelHashes returns the stored channels.access_hash for each id.
	ChannelHashes(ctx context.Context, ids []int64) (map[int64][]int64, error)
	// MessageHasUser reports whether viewer can see msgID in peer and the
	// message refers to userID.
	MessageHasUser(ctx context.Context, viewer int64, peer *mtproto.InputPeer, msgID int32, userID int64) (bool, error)
	// MessageHasChannel reports whether viewer can see msgID in peer and the
	// message refers to channelID.
	MessageHasChannel(ctx context.Context, viewer int64, peer *mtproto.InputPeer, msgID int32, channelID int64) (bool, error)
}

// modeOverride is 0 to read the environment, otherwise mode+1.
var modeOverride atomic.Int32

// Active reports the mode in force. The environment is read on each call so
// a process restart is not required to flip it, and tests can pin a mode.
func Active() Mode {
	if over := modeOverride.Load(); over != 0 {
		return Mode(over - 1)
	}
	return ParseMode(os.Getenv(EnvEnforce))
}

// SetMode pins the mode until the returned function restores the previous pin.
// Tests must defer that function. Production reads the environment.
func SetMode(m Mode) func() {
	prev := modeOverride.Swap(int32(m) + 1)
	return func() { modeOverride.Store(prev) }
}

// ParseMode maps the environment value. Anything unrecognized is off.
func ParseMode(v string) Mode {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "0", "false", "off", "no":
		return ModeOff
	case "log":
		return ModeLog
	case "1", "true", "on", "enforce", "yes":
		return ModeEnforce
	default:
		return ModeOff
	}
}

// Match reports whether a and b are the same access_hash.
func Match(a, b int64) bool {
	var x, y [8]byte
	binary.BigEndian.PutUint64(x[:], uint64(a))
	binary.BigEndian.PutUint64(y[:], uint64(b))
	return subtle.ConstantTimeCompare(x[:], y[:]) == 1
}

// Accept reports whether provided equals one of the hashes the server issued.
func Accept(provided int64, issued []int64) bool {
	if len(issued) == 0 {
		return false
	}
	got := 0
	for _, h := range issued {
		var x, y [8]byte
		binary.BigEndian.PutUint64(x[:], uint64(provided))
		binary.BigEndian.PutUint64(y[:], uint64(h))
		got |= subtle.ConstantTimeCompare(x[:], y[:])
	}
	return got == 1
}

// AllowProvided is the secret-chat comparison.
// Off and log keep the historical rule: a zero on either side skips the
// comparison. Enforce requires an exact match, including rejecting 0 when
// the stored hash is not 0.
func AllowProvided(provided, stored int64) bool {
	if Active() == ModeEnforce {
		return Match(provided, stored)
	}
	if provided != 0 && stored != 0 && provided != stored {
		return false
	}
	return true
}

// AllowChannel is the channel lookup comparison.
// Off and log keep the historical channel rule: a provided hash of 0 skips
// the comparison, and any other provided hash must equal the stored row.
// A stored 0 still rejects a non-zero provided hash. Enforce requires an
// exact match, including rejecting a provided 0 when the stored hash is not 0.
func AllowChannel(provided, stored int64) bool {
	if Active() == ModeEnforce {
		return Match(provided, stored)
	}
	if provided != 0 && provided != stored {
		return false
	}
	return true
}

// IsInputChannelFromMessage reports inputChannelFromMessage, which carries
// no access_hash of its own.
func IsInputChannelFromMessage(in *mtproto.InputChannel) bool {
	if in == nil {
		return false
	}
	return named(in.GetPredicateName(), in.GetConstructor(), mtproto.Predicate_inputChannelFromMessage, mtproto.CRC32_inputChannelFromMessage)
}

// IsInputUserFromMessage reports inputUserFromMessage.
func IsInputUserFromMessage(in *mtproto.InputUser) bool {
	if in == nil {
		return false
	}
	return named(in.GetPredicateName(), in.GetConstructor(), mtproto.Predicate_inputUserFromMessage, mtproto.CRC32_inputUserFromMessage)
}

// Check walks req and applies the active mode.
// viewer 0 (an RPC with no logged-in user) is skipped.
// log receives one line per mismatch or store failure. It may be nil.
// In enforce mode a mismatch returns Kind.Error. A store failure returns
// INTERNAL_SERVER_ERROR. Log mode records the same events and returns nil.
func Check(ctx context.Context, viewer int64, method string, req proto.Message, book Book, log func(string)) error {
	if Active() == ModeOff || viewer == 0 || req == nil {
		return nil
	}
	subjects := Collect(req)
	if len(subjects) == 0 {
		return nil
	}
	if book == nil {
		return failStore(method, "book", log)
	}
	users, channels, err := loadHashes(ctx, viewer, book, subjects)
	if err != nil {
		return failStore(method, err.Error(), log)
	}
	var reject error
	// Hash-bearing constructors first, so a bad container peer is reported
	// before the from-message proof is considered.
	for _, s := range subjects {
		if s.Kind != KindUser && s.Kind != KindPeerUser && s.Kind != KindChannel && s.Kind != KindPeerChannel {
			continue
		}
		if hashOK(s, users, channels) {
			continue
		}
		note(log, method, viewer, s)
		if reject == nil {
			reject = s.Kind.Error()
		}
	}
	for _, s := range subjects {
		switch s.Kind {
		case KindUserFromMessage, KindPeerUserFromMessage:
			ok, err := book.MessageHasUser(ctx, viewer, s.Peer, s.MsgID, s.ID)
			if err != nil {
				return failStore(method, err.Error(), log)
			}
			if ok {
				continue
			}
			note(log, method, viewer, s)
			if reject == nil {
				reject = s.Kind.Error()
			}
		case KindChannelFromMessage, KindPeerChannelFromMessage:
			ok, err := book.MessageHasChannel(ctx, viewer, s.Peer, s.MsgID, s.ID)
			if err != nil {
				return failStore(method, err.Error(), log)
			}
			if ok {
				continue
			}
			note(log, method, viewer, s)
			if reject == nil {
				reject = s.Kind.Error()
			}
		}
	}
	if Active() == ModeEnforce {
		return reject
	}
	return nil
}

func hashOK(s Subject, users, channels map[int64][]int64) bool {
	switch s.Kind {
	case KindUser, KindPeerUser:
		return Accept(s.Provided, users[s.ID])
	case KindChannel, KindPeerChannel:
		return Accept(s.Provided, channels[s.ID])
	default:
		return false
	}
}

func loadHashes(ctx context.Context, viewer int64, book Book, subjects []Subject) (map[int64][]int64, map[int64][]int64, error) {
	var userIDs, channelIDs []int64
	seenU := map[int64]struct{}{}
	seenC := map[int64]struct{}{}
	for _, s := range subjects {
		switch s.Kind {
		case KindUser, KindPeerUser:
			if _, ok := seenU[s.ID]; ok {
				continue
			}
			seenU[s.ID] = struct{}{}
			userIDs = append(userIDs, s.ID)
		case KindChannel, KindPeerChannel:
			if _, ok := seenC[s.ID]; ok {
				continue
			}
			seenC[s.ID] = struct{}{}
			channelIDs = append(channelIDs, s.ID)
		}
	}
	users := map[int64][]int64{}
	channels := map[int64][]int64{}
	var err error
	if len(userIDs) > 0 {
		users, err = book.UserHashes(ctx, viewer, userIDs)
		if err != nil {
			return nil, nil, err
		}
		if users == nil {
			users = map[int64][]int64{}
		}
	}
	if len(channelIDs) > 0 {
		channels, err = book.ChannelHashes(ctx, channelIDs)
		if err != nil {
			return nil, nil, err
		}
		if channels == nil {
			channels = map[int64][]int64{}
		}
	}
	return users, channels, nil
}

func note(log func(string), method string, viewer int64, s Subject) {
	if log == nil {
		return
	}
	log("access_hash mismatch method=" + method +
		" viewer=" + strconv.FormatInt(viewer, 10) +
		" kind=" + s.Kind.String() +
		" target=" + strconv.FormatInt(s.ID, 10) +
		" zero=" + strconv.FormatBool(s.Provided == 0) +
		" msg_id=" + strconv.FormatInt(int64(s.MsgID), 10))
}

func failStore(method, detail string, log func(string)) error {
	if log != nil {
		log("access_hash store failure method=" + method + " err=" + detail)
	}
	if Active() != ModeEnforce {
		return nil
	}
	return mtproto.ErrInternalServerError
}
