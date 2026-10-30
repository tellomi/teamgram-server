package accountrest

import (
	"crypto/rand"
	"encoding/binary"
	"sync"

	"github.com/teamgram/proto/mtproto"
)

// Takeout sessions issued by account.initTakeoutSession.
// https://core.telegram.org/api/takeout
// https://core.telegram.org/method/account.initTakeoutSession
// https://core.telegram.org/method/account.finishTakeoutSession
//
// finish without an open session is 403 TAKEOUT_REQUIRED.
// A wrapped takeout id that is not the user's open session is 400 TAKEOUT_INVALID.

type takeoutRec struct {
	id       int64
	userID   int64
	contacts bool
	users    bool
	chats    bool
	megas    bool
	channels bool
	files    bool
	maxSize  int64
	success  bool
	done     bool
}

type takeoutStore struct {
	mu   sync.Mutex
	open map[int64]*takeoutRec
	byID map[int64]*takeoutRec
}

var takeouts = &takeoutStore{
	open: map[int64]*takeoutRec{},
	byID: map[int64]*takeoutRec{},
}

func resetTakeouts() {
	takeouts.mu.Lock()
	takeouts.open = map[int64]*takeoutRec{}
	takeouts.byID = map[int64]*takeoutRec{}
	takeouts.mu.Unlock()
}

// InitTakeoutSession opens one export session for the user and returns account.takeout.
// A previous open session for the same user is replaced.
func InitTakeoutSession(userID int64, in *mtproto.TLAccountInitTakeoutSession) (*mtproto.Account_Takeout, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	if in == nil {
		in = &mtproto.TLAccountInitTakeoutSession{}
	}
	id, err := newTakeoutID()
	if err != nil {
		return nil, err
	}
	rec := &takeoutRec{
		id:       id,
		userID:   userID,
		contacts: in.GetContacts(),
		users:    in.GetMessageUsers(),
		chats:    in.GetMessageChats(),
		megas:    in.GetMessageMegagroups(),
		channels: in.GetMessageChannels(),
		files:    in.GetFiles(),
		maxSize:  fileMaxSize(in),
	}
	takeouts.mu.Lock()
	if prev := takeouts.open[userID]; prev != nil {
		prev.done = true
		delete(takeouts.byID, prev.id)
	}
	takeouts.open[userID] = rec
	takeouts.byID[id] = rec
	takeouts.mu.Unlock()
	return mtproto.MakeTLAccountTakeout(&mtproto.Account_Takeout{Id: id}).To_Account_Takeout(), nil
}

// FinishTakeoutSession closes the session. takeoutID is the invokeWithTakeout id
// when the client wrapped the call; 0 finishes the user's open session.
func FinishTakeoutSession(userID, takeoutID int64, success bool) (*mtproto.Bool, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	takeouts.mu.Lock()
	defer takeouts.mu.Unlock()
	rec := takeouts.open[userID]
	if takeoutID != 0 {
		got := takeouts.byID[takeoutID]
		if got == nil || got.userID != userID || got.done {
			return nil, mtproto.ErrTakeoutInvalid
		}
		rec = got
	}
	if rec == nil || rec.done {
		return nil, mtproto.ErrForbiddenTakeoutRequired
	}
	rec.done = true
	rec.success = success
	delete(takeouts.open, userID)
	delete(takeouts.byID, rec.id)
	return mtproto.BoolTrue, nil
}

func fileMaxSize(in *mtproto.TLAccountInitTakeoutSession) int64 {
	if !in.GetFiles() {
		return 0
	}
	if v := in.GetFileMaxSize_FLAGINT64(); v != nil {
		return v.GetValue()
	}
	if v := in.GetFileMaxSize_FLAGINT32(); v != nil {
		return int64(v.GetValue())
	}
	return 0
}

func newTakeoutID() (int64, error) {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 0, err
	}
	id := int64(binary.BigEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
	if id == 0 {
		return 1, nil
	}
	return id, nil
}
