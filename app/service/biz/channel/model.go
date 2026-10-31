// Copyright 2026 Tellomi
//
// Channels and supergroups. Message ids and pts are per channel, matching
// Telegram: a new message, an edit, and a delete each advance that channel's
// pts and show up in updates.getChannelDifference. User pts is not used.

package channel

import "github.com/teamgram/proto/mtproto"

const (
	RoleMember  int32 = 0
	RoleCreator int32 = 1
	RoleAdmin   int32 = 2
	RoleBanned  int32 = 4
)

const (
	StateNormal int32 = 0
	StateLeft   int32 = 1
	StateKicked int32 = 2
)

const (
	KindNew    = "new"
	KindEdit   = "edit"
	KindDelete = "delete"
)

// Channel is the stored channel or supergroup.
type Channel struct {
	ID                 int64
	AccessHash         int64
	CreatorID          int64
	Title              string
	About              string
	Username           string
	PhotoID            int64
	Broadcast          bool
	Megagroup          bool
	Forum              bool
	Signatures         bool
	Pts                int32
	TopMessage         int32
	ParticipantsCount  int32
	Date               int32
	MigratedFromChatID int64
	MigratedFromMaxID  int32
	TtlPeriod          int32
	Address            string
	DefaultBanned      int64
	Deleted            bool
	Version            int32
}

func (c *Channel) clone() *Channel {
	if c == nil {
		return nil
	}
	cp := *c
	return &cp
}

func (c *Channel) Public() bool {
	return c != nil && c.Username != ""
}

// Participant is one user's membership.
type Participant struct {
	ChannelID    int64
	UserID       int64
	Role         int32
	State        int32
	AdminRights  int32
	BannedRights int64
	Rank         string
	InviterID    int64
	IsBot        bool
	Date         int32
}

func (p *Participant) clone() *Participant {
	if p == nil {
		return nil
	}
	cp := *p
	return &cp
}

func (p *Participant) IsCreator() bool {
	return p != nil && p.Role == RoleCreator && p.State == StateNormal
}

func (p *Participant) IsAdmin() bool {
	return p != nil && p.State == StateNormal && (p.Role == RoleCreator || p.Role == RoleAdmin)
}

func (p *Participant) InChannel() bool {
	return p != nil && p.State == StateNormal
}

func (p *Participant) adminRights() mtproto.AdminRights {
	if p == nil {
		return 0
	}
	if p.Role == RoleCreator {
		return mtproto.AdminRights(allAdminRights())
	}
	return mtproto.AdminRights(p.AdminRights)
}

// StoredMessage is one channel message. Data is a marshaled mtproto.Message
// with Out cleared; callers set Out for the viewer.
type StoredMessage struct {
	ChannelID int64
	ID        int32
	SenderID  int64
	Date      int32
	EditDate  int32
	RandomID  int64
	Pts       int32
	Deleted   bool
	Data      []byte
}

func (m *StoredMessage) clone() *StoredMessage {
	if m == nil {
		return nil
	}
	cp := *m
	if m.Data != nil {
		cp.Data = append([]byte(nil), m.Data...)
	}
	return &cp
}

// PtsEvent is one channel pts step. Pts is the channel pts after the event.
type PtsEvent struct {
	ChannelID  int64
	Pts        int32
	PtsCount   int32
	Kind       string
	MessageID  int32
	MessageIDs []int32
	Payload    []byte
}

func (e *PtsEvent) clone() *PtsEvent {
	if e == nil {
		return nil
	}
	cp := *e
	if e.MessageIDs != nil {
		cp.MessageIDs = append([]int32(nil), e.MessageIDs...)
	}
	if e.Payload != nil {
		cp.Payload = append([]byte(nil), e.Payload...)
	}
	return &cp
}

// ReadMark is one member's read time for messages up to MaxID.
type ReadMark struct {
	UserID int64
	MaxID  int32
	Date   int64
}

// ReadState is one user's read cursor inside a channel.
type ReadState struct {
	UserID          int64
	ChannelID       int64
	ReadInboxMaxID  int32
	ReadOutboxMaxID int32
	UnreadCount     int32
}

func (r *ReadState) clone() *ReadState {
	if r == nil {
		return nil
	}
	cp := *r
	return &cp
}

// HistoryQuery matches messages.getHistory / channels.getMessages paging.
type HistoryQuery struct {
	OffsetID   int32
	OffsetDate int32
	AddOffset  int32
	Limit      int32
	MaxID      int32
	MinID      int32
}
