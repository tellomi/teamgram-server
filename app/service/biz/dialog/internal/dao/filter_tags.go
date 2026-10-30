package dao

import (
	"context"
	"errors"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
)

// dialogFilterTagsKey is the user_settings row for messages.dialogFilters.tags_enabled.
// https://core.telegram.org/constructor/messages.dialogFilters
const dialogFilterTagsKey = "dialog_filter_tags"

type dialogFilterTagsRow struct {
	Value string `db:"value"`
}

// GetDialogFilterTags reports whether folder tags are enabled.
// A missing row is the documented default: tags are off.
func (d *Dao) GetDialogFilterTags(ctx context.Context, userID int64) (bool, error) {
	row := &dialogFilterTagsRow{}
	err := d.QueryRowPartial(ctx, row,
		"select value from user_settings where user_id = ? and key2 = ? and deleted = 0 limit 1",
		userID, dialogFilterTagsKey)
	if err != nil {
		if errors.Is(err, sqlx.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return row.Value == "true", nil
}

// SetDialogFilterTags stores the folder-tags toggle on the shared user_settings table.
func (d *Dao) SetDialogFilterTags(ctx context.Context, userID int64, enabled bool) error {
	value := "false"
	if enabled {
		value = "true"
	}
	_, err := d.Mysql.Exec(ctx,
		"insert into user_settings (user_id, key2, value) values (?, ?, ?) on duplicate key update value = values(value), deleted = 0",
		userID, dialogFilterTagsKey, value)
	return err
}
