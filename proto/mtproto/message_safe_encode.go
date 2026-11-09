package mtproto

import (
	"fmt"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Encode serializes one message. Entities are filtered again here so a push,
// getHistory, or getDifference cannot skip the UTF-16 check. A panic or
// encode error in this message is rolled back and the message is degraded
// (entities removed, then messageEmpty). The caller keeps going, which is
// what lets getDifference still write its state and advance pts.
func (m *Message) Encode(x *EncodeBuf, layer int32) (err error) {
	if m == nil {
		return fmt.Errorf("nil message")
	}
	// views and forwards share one flag. The decoder reads both integers
	// when that flag is set. A post that only has a view count has to send
	// forwards as 0, or the next vector is consumed as the forward count.
	if m.GetViews() != nil && m.GetForwards() == nil {
		m.Forwards = &wrapperspb.Int32Value{Value: 0}
	}
	if m.GetForwards() != nil && m.GetViews() == nil {
		m.Views = &wrapperspb.Int32Value{Value: 0}
	}
	m.SanitizeEntities()
	start := x.GetOffset()
	defer func() {
		if r := recover(); r != nil {
			x.Truncate(start)
			err = m.encodeDegraded(x, layer)
		}
	}()
	err = m.encodeMessageBody(x, layer)
	if err != nil {
		x.Truncate(start)
		err = m.encodeDegraded(x, layer)
	}
	return err
}

// encodeDegraded retries without entities, then as messageEmpty. Either
// result is a complete object, or nothing is written and err is returned so
// the surrounding vector does not count a partial message.
func (m *Message) encodeDegraded(x *EncodeBuf, layer int32) error {
	id := m.GetId()
	if len(m.Entities) > 0 {
		saved := m.Entities
		m.Entities = nil
		start := x.GetOffset()
		err := func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("message encode panic: %v", r)
				}
			}()
			return m.encodeMessageBody(x, layer)
		}()
		m.Entities = saved
		if err == nil {
			return nil
		}
		x.Truncate(start)
	}
	empty := MakeTLMessageEmpty(&Message{Id: id}).To_Message()
	start := x.GetOffset()
	err := func() (err error) {
		defer func() {
			if r := recover(); r != nil {
				err = fmt.Errorf("messageEmpty encode panic: %v", r)
			}
		}()
		return empty.encodeMessageBody(x, layer)
	}()
	if err != nil {
		x.Truncate(start)
	}
	return err
}

// Encode filters embedded entities, then serializes the update. A panic is
// discarded and reported as an error with the buffer restored, so one bad
// update in a getDifference vector does not abort the rest of the reply.
func (m *Update) Encode(x *EncodeBuf, layer int32) (err error) {
	if m == nil {
		return fmt.Errorf("nil update")
	}
	m.SanitizeEmbeddedEntities()
	start := x.GetOffset()
	defer func() {
		if r := recover(); r != nil {
			x.Truncate(start)
			err = fmt.Errorf("update encode panic: %v", r)
		}
	}()
	err = m.encodeUpdateBody(x, layer)
	if err != nil {
		x.Truncate(start)
	}
	return err
}

// Encode filters short-message entities before serialization. Pushes use
// this path, as do updates returned from sendMessage.
func (m *Updates) Encode(x *EncodeBuf, layer int32) error {
	if m == nil {
		return fmt.Errorf("nil updates")
	}
	m.SanitizeOutgoing()
	return m.encodeUpdatesBody(x, layer)
}
