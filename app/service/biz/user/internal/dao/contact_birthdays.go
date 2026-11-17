package dao

import (
	"context"

	"github.com/teamgram/proto/mtproto"
)

// ContactBirthdays lists address-book contacts who have a birthday stored.
// contacts.getBirthdays is that list. A contact with an empty birthday is omitted.
func (d *Dao) ContactBirthdays(ctx context.Context, userID int64) ([]*mtproto.ContactBirthday, error) {
	if d == nil || d.DB == nil || userID == 0 {
		return nil, nil
	}
	var rows []struct {
		ContactUserID int64  `db:"contact_user_id"`
		Birthday      string `db:"birthday"`
	}
	err := d.DB.QueryRowsPartial(ctx, &rows, `select c.contact_user_id, u.birthday
		from user_contacts c
		join users u on u.id = c.contact_user_id
		where c.owner_user_id = ? and c.is_deleted = 0 and u.deleted = 0 and u.birthday <> ''`, userID)
	if err != nil {
		return nil, err
	}
	out := make([]*mtproto.ContactBirthday, 0, len(rows))
	for _, row := range rows {
		bday := mtproto.FromBirthdayString(row.Birthday)
		if bday == nil || bday.GetMonth() == 0 || bday.GetDay() == 0 {
			continue
		}
		out = append(out, mtproto.MakeTLContactBirthday(&mtproto.ContactBirthday{
			ContactId: row.ContactUserID,
			Birthday:  bday,
		}).To_ContactBirthday())
	}
	return out, nil
}
