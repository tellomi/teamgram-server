package dao

import (
	"context"
	"errors"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/media/media"
)

// hidePreviousFallbackSQL retires every other public photo. deleted = 1
// removes it from the album. photo_id <> ? keeps the photo being set.
const hidePreviousFallbackSQL = `update user_profile_photos
	set deleted = 1, date2 = 0, fallback = 0
	where user_id = ? and fallback = 1 and deleted = 0 and photo_id <> ?`

// UpdateFallbackPhoto stores photoId as the public avatar shown when profile
// photo privacy hides the main picture. It does not replace the main photo.
// The previous public photo is deleted from the album.
func (d *Dao) UpdateFallbackPhoto(ctx context.Context, userID, photoID int64) int64 {
	if d == nil || userID == 0 || photoID <= 0 {
		return 0
	}
	_, _, err := d.CachedConn.Exec(ctx, func(ctx context.Context, conn *sqlx.DB) (int64, int64, error) {
		tR := sqlx.TxWrapper(ctx, conn, func(tx *sqlx.Tx, result *sqlx.StoreResult) {
			// The previous public photo is removed. Clearing only the flag
			// would leave it in the album. The main avatar has fallback = 0,
			// so this update does not touch it, and users.photo_id is not written.
			_, result.Err = tx.Exec(hidePreviousFallbackSQL, userID, photoID)
			if result.Err != nil {
				return
			}
			_, result.Err = tx.Exec(`insert into user_profile_photos (user_id, photo_id, date2, deleted, fallback)
				values (?, ?, ?, 0, 1)
				on duplicate key update date2 = values(date2), deleted = 0, fallback = 1`,
				userID, photoID, time.Now().Unix())
		})
		if tR.Err != nil {
			return 0, 0, tR.Err
		}
		return 0, 1, nil
	}, genCacheUserDataCacheKey(userID))
	if err != nil {
		return 0
	}
	return photoID
}

// FallbackPhotoID is the public avatar, or 0 when the user has not set one.
func (d *Dao) FallbackPhotoID(ctx context.Context, userID int64) (int64, error) {
	if d == nil || d.DB == nil || userID == 0 {
		return 0, nil
	}
	var id int64
	err := d.DB.QueryRowPartial(ctx, &id, `select photo_id from user_profile_photos
		where user_id = ? and deleted = 0 and fallback = 1 order by date2 desc limit 1`, userID)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return id, nil
}

// AlbumPhotoIDs lists profile photos that are not the public fallback.
func (d *Dao) AlbumPhotoIDs(ctx context.Context, userID int64) ([]int64, error) {
	if d == nil || d.DB == nil || userID == 0 {
		return nil, nil
	}
	var ids []int64
	err := d.DB.QueryRowsPartial(ctx, &ids, `select photo_id from user_profile_photos
		where user_id = ? and deleted = 0 and fallback = 0 order by date2 desc`, userID)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return ids, nil
}

// NextAlbumPhoto is the newest non-fallback photo other than excluded.
func (d *Dao) NextAlbumPhoto(ctx context.Context, userID int64, excluded []int64) (int64, error) {
	if d == nil || d.DB == nil || userID == 0 || len(excluded) == 0 {
		return 0, nil
	}
	query := `select photo_id from user_profile_photos
		where user_id = ? and deleted = 0 and fallback = 0 and photo_id not in (` + sqlx.InInt64List(excluded) + `)
		order by date2 desc limit 1`
	var id int64
	err := d.DB.QueryRowPartial(ctx, &id, query, userID)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return id, nil
}

// FallbackPhoto loads the public avatar. A missing one is an empty photo.
func (d *Dao) FallbackPhoto(ctx context.Context, userID int64) (*mtproto.Photo, error) {
	id, err := d.FallbackPhotoID(ctx, userID)
	if err != nil || id == 0 {
		return mtproto.MakeTLPhotoEmpty(&mtproto.Photo{}).To_Photo(), err
	}
	if d.MediaClient == nil {
		return mtproto.MakeTLPhotoEmpty(&mtproto.Photo{}).To_Photo(), nil
	}
	photo, err := d.MediaClient.MediaGetPhoto(ctx, &media.TLMediaGetPhoto{PhotoId: id})
	if err != nil || photo == nil {
		return mtproto.MakeTLPhotoEmpty(&mtproto.Photo{}).To_Photo(), err
	}
	return photo, nil
}
