package story

import "context"

// Store persists stories. Create assigns a story id when Item.Id is 0.
type Store interface {
	Create(ctx context.Context, rec *Record) error
	Save(ctx context.Context, rec *Record) error
	Get(ctx context.Context, owner Owner, id int32) (*Record, error)
	FindByRandom(ctx context.Context, owner Owner, sender, randomID int64) (*Record, error)
	ListByOwner(ctx context.Context, owner Owner) ([]*Record, error)
	ListAll(ctx context.Context) ([]*Record, error)
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
}
