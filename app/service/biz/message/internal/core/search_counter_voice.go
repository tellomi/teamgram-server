package core

import (
	"fmt"

	"github.com/teamgram/proto/mtproto"
)

// message_data is a JSON column. MySQL stores it in binary form and the
// text used by LIKE has a space after the colon ("voice": true). The bytes
// written by encoding/json have no space ("voice":true). omitempty drops a
// false bool, so a music file does not contain the voice mark.
const (
	voiceDataMark = `"voice": true`
	roundDataMark = `"round\\_message": true`
)

// audioKindWhere is the WHERE of SELECT count(id), the same statement
// photos use. It only matches MEDIA_AUDIO, so a row stored as
// MEDIA_VOICE_FILE or MEDIA_ROUND_FILE is not counted here as well.
// Those columns are added by countMediaTypes. The two literals are fixed;
// the ids are integers.
func audioKindWhere(userID, dialogA, dialogB int64, voice bool) string {
	mark := roundDataMark
	if voice {
		mark = voiceDataMark
	}
	return fmt.Sprintf(
		"user_id = %d AND dialog_id1 = %d AND dialog_id2 = %d AND message_filter_type = %d AND deleted = 0 AND message_data LIKE '%%%s%%'",
		userID, dialogA, dialogB, mtproto.MEDIA_AUDIO, mark,
	)
}
