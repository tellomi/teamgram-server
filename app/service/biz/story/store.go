package story

import "context"

// Store persists stories. Create assigns a story id when Item.Id is 0.
type Store interface {
	Create(ctx context.Context, rec *Record) error
	Save(ctx context.Context, rec *Record) error
	Get(ctx context.Context, owner Owner, id int32) (*Record, error)
	FindByRandom(ctx context.Context, owner Owner, sender, randomID int64) (*Record, error)
	ListByOwner(ctx context.Context, owner Owner) ([]*Record, error)
	// ListActiveByOwners returns non-deleted stories with expire_date > now
	// for the given owners of one kind, in one read.
	ListActiveByOwners(ctx context.Context, kind Kind, ids []int64, now int32) ([]*Record, error)
	ListAll(ctx context.Context) ([]*Record, error)
	// OwnersExpiredBetween lists peers with a non-deleted story whose
	// expire_date is after `after` and at or before `now`.
	OwnersExpiredBetween(ctx context.Context, after, now int32) ([]Owner, error)
	// NextExpiry is the soonest expire_date strictly after now, or 0.
	NextExpiry(ctx context.Context, now int32) (int32, error)
	AddForwards(ctx context.Context, owner Owner, id int32) error

	SaveView(ctx context.Context, owner Owner, storyID int32, v View) error
	ListViews(ctx context.Context, owner Owner, storyID int32) ([]View, error)
	HideActiveViews(ctx context.Context, viewer int64, now int32) error

	GetRead(ctx context.Context, userID int64, owner Owner) (*Read, error)
	SaveRead(ctx context.Context, rd *Read) error
	ListReads(ctx context.Context, userID int64) ([]*Read, error)

	GetSettings(ctx context.Context, userID int64) (*Settings, error)
	SaveSettings(ctx context.Context, st *Settings) error

	CreateAlbum(ctx context.Context, a *Album) error
	SaveAlbum(ctx context.Context, a *Album) error
	GetAlbum(ctx context.Context, owner Owner, id int32) (*Album, error)
	ListAlbums(ctx context.Context, owner Owner) ([]*Album, error)
	DeleteAlbum(ctx context.Context, owner Owner, id int32) error

	SetPinnedTop(ctx context.Context, owner Owner, ids []int32) error
	PinnedTop(ctx context.Context, owner Owner) ([]int32, error)

	SaveReport(ctx context.Context, r *Report) error

	// SaveDeletions records that each viewer still needs updateStory with
	// storyItemDeleted for this story. A repeat is idempotent.
	SaveDeletions(ctx context.Context, owner Owner, storyID, deletedAt int32, viewers []int64) error
	// ListDeletions returns deletions with deleted_at > since, oldest first.
	ListDeletions(ctx context.Context, viewer int64, since int32, limit int) ([]Deletion, error)
	// DeletionMarks is one watermark per owner this viewer was told about.
	DeletionMarks(ctx context.Context, viewer int64) ([]DeletionMark, error)

	// StoriesByMedia lists stories whose photo or document id is mediaID.
	// An empty list means the file is not story media.
	StoriesByMedia(ctx context.Context, mediaID int64) ([]MediaRef, error)
}

// MediaRef is one story that stores a photo or document.
type MediaRef struct {
	Owner   Owner
	StoryID int32
}
