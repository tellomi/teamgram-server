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
	FindByMigratedChat(ctx context.Context, chatID int64) (*Channel, error)
	ListChannels(ctx context.Context, ids []int64) ([]*Channel, error)
	ListByUser(ctx context.Context, userID int64) ([]*Channel, error)

	PutParticipant(ctx context.Context, p *Participant) error
	GetParticipant(ctx context.Context, channelID, userID int64) (*Participant, error)
	ListParticipants(ctx context.Context, channelID int64) ([]*Participant, error)
	CountMembers(ctx context.Context, channelID int64) (int32, error)

	CommitNew(ctx context.Context, channelID, senderID, randomID int64, date int32, build BuildMessage) (messageID, pts int32, reused bool, err error)
	CommitEdit(ctx context.Context, channelID int64, messageID int32, editDate int32, data []byte) (pts int32, err error)
	CommitDelete(ctx context.Context, channelID int64, ids []int32) (pts, ptsCount int32, deleted []int32, err error)
	GetMessage(ctx context.Context, channelID int64, id int32) (*StoredMessage, error)
	ListMessages(ctx context.Context, channelID int64) ([]*StoredMessage, error)
	ListPtsAfter(ctx context.Context, channelID int64, pts int32, limit int) ([]*PtsEvent, error)
	AppendPts(ctx context.Context, channelID int64, kind string, messageID int32, payload []byte) (pts int32, err error)

	GetRead(ctx context.Context, channelID, userID int64) (*ReadState, error)
	SaveRead(ctx context.Context, st *ReadState) error

	GetFeatures(ctx context.Context, channelID int64) (*Features, error)
	SaveFeatures(ctx context.Context, channelID int64, f *Features) error
	GetPersonalChannel(ctx context.Context, userID int64) (int64, error)
	SetPersonalChannel(ctx context.Context, userID, channelID int64) error
}
