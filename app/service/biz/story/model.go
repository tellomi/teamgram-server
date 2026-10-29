package story

import "github.com/teamgram/proto/mtproto"

// Kind is the peer that owns a story. Users and channels both post stories.
type Kind int8

const (
	KindUser    Kind = 1
	KindChannel Kind = 2
)

// Owner is the profile a story is posted to.
type Owner struct {
	Kind Kind
	ID   int64
}

func (o Owner) Peer() *mtproto.Peer {
	if o.Kind == KindChannel {
		return mtproto.MakePeerChannel(o.ID)
	}
	return mtproto.MakePeerUser(o.ID)
}

func (o Owner) ok() bool {
	return o.ID != 0 && (o.Kind == KindUser || o.Kind == KindChannel)
}

// Record is one stored story. Views, albums, and per-viewer reactions are
// stored beside it and applied when the story is rendered.
type Record struct {
	Owner    Owner
	Sender   int64
	RandomID int64
	Forwards int32
	Deleted  bool
	Item     *mtproto.StoryItem
}

func (r *Record) id() int32 {
	if r == nil || r.Item == nil {
		return 0
	}
	return r.Item.Id
}

func (r *Record) active(now int32) bool {
	return r != nil && !r.Deleted && r.Item != nil && r.Item.ExpireDate > now
}

// View is one user opening a story. Stealth views are stored and then omitted
// from counts and viewer lists.
type View struct {
	UserID   int64
	Date     int32
	Reaction string
	Stealth  bool
}

// Read is how far a user has read one peer, and whether that peer is hidden.
type Read struct {
	UserID int64
	Owner  Owner
	MaxID  int32
	Hidden bool
}

// Settings is per-viewer story state: the hidden archive and stealth mode.
type Settings struct {
	UserID          int64
	AllHidden       bool
	StealthActive   int32
	StealthCooldown int32
}

// Album is an ordered profile collection of story ids.
type Album struct {
	Owner    Owner
	ID       int32
	Title    string
	Order    int32
	StoryIDs []int32
}

// Report is a stored stories.report submission.
type Report struct {
	Reporter int64
	Owner    Owner
	StoryIDs []int32
	Option   string
	Message  string
	Date     int32
}

const (
	defaultPeriod     int32 = 86400
	dailyLimit        int32 = 100
	stealthFutureSecs int32 = 25 * 60
	stealthCooldown   int32 = 3 * 3600
	maxPinnedTop            = 3
)
