package channel

import "context"

// BuildMessage runs inside the store lock or transaction. messageID and pts
// are the values just allocated for this message.
type BuildMessage func(messageID, pts int32) []byte

// Store persists channels. Implementations must make Commit* atomic with the
// channel pts counter.
type Store interface {
	AllocID(ctx context.Context) (int64, error)
	CreateChannel(ctx context.Context, ch *Channel, creator *Participant) error
	GetChannel(ctx context.Context, id int64) (*Channel, error)
	SaveChannel(ctx context.Context, ch *Channel) error
	FindByUsername(ctx context.Context, username string) (*Channel, error)
	// LookupUsername reports whether username is reserved by a channel.
	// active is false when the name is kept but not the public link.
	LookupUsername(ctx context.Context, username string) (channelID int64, active, found bool, err error)
	ListPublic(ctx context.Context) ([]*Channel, error)
	ListLeftByUser(ctx context.Context, userID int64) ([]*Channel, error)
	FindByMigratedChat(ctx context.Context, chatID int64) (*Channel, error)
	ListChannels(ctx context.Context, ids []int64) ([]*Channel, error)
	ListByUser(ctx context.Context, userID int64) ([]*Channel, error)
	// SearchPosts returns channel posts newest first. Limit is the maximum
	// number of rows. The caller asks for one extra row to know the page continues.
	SearchPosts(ctx context.Context, self int64, q PostQuery) ([]*StoredMessage, error)
	// CountJoinedText is the number of joined posts whose text contains needle.
	// restrict limits the count to broadcasts and/or megagroups.
	CountJoinedText(ctx context.Context, self int64, needle string, restrict, broadcasts, megagroups bool) (int32, error)
	// SearchPublic returns public channels and groups whose title or username
	// matches q. Deleted, banned, and restricted channels are left out, and so
	// is a channel that banned or kicked self.
	SearchPublic(ctx context.Context, self int64, q string, limit int32) ([]*Channel, error)

	PutParticipant(ctx context.Context, p *Participant) error
	GetParticipant(ctx context.Context, channelID, userID int64) (*Participant, error)
	ListParticipants(ctx context.Context, channelID int64) ([]*Participant, error)
	CountMembers(ctx context.Context, channelID int64) (int32, error)

	CommitNew(ctx context.Context, channelID, senderID, randomID int64, date int32, build BuildMessage) (messageID, pts int32, reused bool, err error)
	CommitEdit(ctx context.Context, channelID int64, messageID int32, editDate int32, data []byte) (pts int32, err error)
	CommitDelete(ctx context.Context, channelID int64, ids []int32) (pts, ptsCount int32, deleted []int32, err error)
	GetMessage(ctx context.Context, channelID int64, id int32) (*StoredMessage, error)
	// LatestLiveMessage is the newest message at or below atOrBelow that is
	// not deleted. The channel's top_message counter is not reused: deleting
	// the latest post leaves that counter in place and the dialog shows this
	// id instead. A nil result means nothing at or below atOrBelow is live.
	LatestLiveMessage(ctx context.Context, channelID int64, atOrBelow int32) (*StoredMessage, error)
	// ReplaceMessageBody rewrites a message without a pts edit event.
	// Poll results use it so the message is not marked edited.
	ReplaceMessageBody(ctx context.Context, channelID int64, id int32, data []byte) error
	ListMessages(ctx context.Context, channelID int64) ([]*StoredMessage, error)
	ListPtsAfter(ctx context.Context, channelID int64, pts int32, limit int) ([]*PtsEvent, error)
	AppendPts(ctx context.Context, channelID int64, kind string, messageID int32, payload []byte) (pts int32, err error)

	GetRead(ctx context.Context, channelID, userID int64) (*ReadState, error)
	SaveRead(ctx context.Context, st *ReadState) error
	// ApplyUnreadCuts runs one indexed update per cut. It does not read or
	// write a row per member. list returns the rows that match the cuts
	// after the updates; a large channel passes false and skips that read.
	ApplyUnreadCuts(ctx context.Context, channelID int64, cuts []UnreadCut, list bool) ([]ReadState, error)
	// SaveReadMark records when reader covered messages up to maxID.
	// ownerUserID is the channel id; peerDialogID is its dialog id.
	SaveReadMark(ctx context.Context, ownerUserID, peerDialogID, readerUserID int64, maxID int32, date int64) error
	ListReadMarks(ctx context.Context, ownerUserID, peerDialogID int64, msgID int32) ([]ReadMark, error)

	GetFeatures(ctx context.Context, channelID int64) (*Features, error)
	SaveFeatures(ctx context.Context, channelID int64, f *Features) error
	GetPersonalChannel(ctx context.Context, userID int64) (int64, error)
	SetPersonalChannel(ctx context.Context, userID, channelID int64) error

	InsertInvite(ctx context.Context, inv *StoredInvite) error
	UpdateInvite(ctx context.Context, inv *StoredInvite) error
	GetInviteByLink(ctx context.Context, link string) (*StoredInvite, error)
	ListInvites(ctx context.Context, channelID int64) ([]*StoredInvite, error)
	DeleteInvite(ctx context.Context, channelID int64, link string) error
	DeleteRevokedInvites(ctx context.Context, channelID, adminID int64) error
	UpsertImporter(ctx context.Context, row *StoredImporter) error
	ListImporters(ctx context.Context, channelID int64) ([]*StoredImporter, error)
	DeleteImporter(ctx context.Context, link string, userID int64) error
}

// PostQuery is one page of channels.searchPosts.
// Needle is the raw text, not a LIKE pattern. OffsetRate 0 and OffsetID 0
// start at the newest post. A positive OffsetRate is the previous page's
// date; rows are strictly older in (date, channel, message id) order.
// JoinedOnly limits the scan to channels the user has joined. Public
// channels they have not joined stay out. messages.searchGlobal sets it.
// channels.searchPosts leaves it false.
type PostQuery struct {
	Needle     string
	OffsetRate int32
	OffsetPeer int64
	OffsetID   int32
	Limit      int32
	JoinedOnly bool
	// MaxDate keeps posts at or before this unix time. Zero means no cap.
	// channels.searchPosts paging uses OffsetRate instead.
	MaxDate int32
	// GlobalOrder is messages.searchGlobal. Rows are date DESC, channel id ASC,
	// message id DESC, and CursorOn skips anything that is not strictly after
	// that tuple. It does not use the channels.searchPosts channel-id DESC order.
	GlobalOrder    bool
	CursorOn       bool
	CursorDate     int32
	CursorPeerType int32
	CursorPeer     int64
	CursorMsg      int32
}
