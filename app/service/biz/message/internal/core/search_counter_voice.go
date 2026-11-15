package core

import (
	"fmt"

	"github.com/teamgram/proto/mtproto"
)

// Marks written by encoding/json into message_data. omitempty drops a
// false bool, so a voice note contains "voice":true and a round video
// contains "round_message":true. A music file's audio attribute does not.
const (
	voiceDataMark = `"voice":true`
	roundDataMark = `"round_message":true`
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
