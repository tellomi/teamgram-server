package core

import (
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
)

// maxProfileNameRunes matches users.first_name / users.last_name varchar(64)
// and the official account.updateProfile limit.
const maxProfileNameRunes = 64

// checkedProfileName trims the edges. first requires a non-empty result.
// Either side longer than 64 runes is invalid.
func checkedProfileName(value string, first bool) (string, error) {
	value = strings.TrimSpace(value)
	if utf8.RuneCountInString(value) > maxProfileNameRunes || (first && value == "") {
		if first {
			return "", mtproto.ErrFirstnameInvalid
		}
		return "", mtproto.ErrLastnameInvalid
	}
	return value, nil
}
