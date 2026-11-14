package core

import (
	"github.com/teamgram/proto/mtproto"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// applyEditEntities stores the entities sent with messages.editMessage.
// Replacing the text drops the previous entities, because their offsets
// belong to the old string. The entities flag is applied after that,
// including an empty vector, which removes formatting. A text edit that
// also sends bold keeps those entities.
func applyEditEntities(msg *mtproto.Message, text *wrapperspb.StringValue, entities []*mtproto.MessageEntity) {
	if msg == nil {
		return
	}
	if text != nil {
		msg.Message = text.GetValue()
		msg.Entities = nil
	}
	if entities != nil {
		msg.Entities = entities
	}
}
