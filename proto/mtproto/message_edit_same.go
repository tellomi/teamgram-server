package mtproto

import "google.golang.org/protobuf/proto"

// MessageContentUnchanged reports whether an edit left the text, entities,
// media, and reply markup the same. edit_date is not part of the comparison:
// a no-op edit is MESSAGE_NOT_MODIFIED and must not bump that field.
func MessageContentUnchanged(before, after *Message) bool {
	if before == nil || after == nil {
		return before == after
	}
	if before.GetMessage() != after.GetMessage() {
		return false
	}
	if !proto.Equal(before.GetMedia(), after.GetMedia()) {
		return false
	}
	if !proto.Equal(before.GetReplyMarkup(), after.GetReplyMarkup()) {
		return false
	}
	return entitiesEqual(before.GetEntities(), after.GetEntities())
}

func entitiesEqual(a, b []*MessageEntity) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !proto.Equal(a[i], b[i]) {
			return false
		}
	}
	return true
}
