package session

import (
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// These field numbers are free on the generated push messages (mtprotoc did
// not assign them). Sync stamps the user the update is for; session reads it
// back before writing the update onto a connection. The bytes travel as
// unknown fields so the generated descriptors stay untouched. A later
// descriptor that claims field 6 or 7 has to move these constants with it.

const (
	pushUpdatesUserField        protowire.Number = 6
	pushSessionUpdatesUserField protowire.Number = 7
)

// SetPushUpdatesUser records the user an updates push is for.
func SetPushUpdatesUser(msg *TLSessionPushUpdatesData, userID int64) {
	if msg == nil {
		return
	}
	setUserField(msg, pushUpdatesUserField, userID)
}

// PushUpdatesUser returns the user stamped by SetPushUpdatesUser.
func PushUpdatesUser(msg *TLSessionPushUpdatesData) int64 {
	if msg == nil {
		return 0
	}
	return userField(msg, pushUpdatesUserField)
}

// SetPushSessionUpdatesUser records the user a single-session push is for.
func SetPushSessionUpdatesUser(msg *TLSessionPushSessionUpdatesData, userID int64) {
	if msg == nil {
		return
	}
	setUserField(msg, pushSessionUpdatesUserField, userID)
}

// PushSessionUpdatesUser returns the user stamped by SetPushSessionUpdatesUser.
func PushSessionUpdatesUser(msg *TLSessionPushSessionUpdatesData) int64 {
	if msg == nil {
		return 0
	}
	return userField(msg, pushSessionUpdatesUserField)
}

func setUserField(msg interface{ ProtoReflect() protoreflect.Message }, num protowire.Number, userID int64) {
	r := msg.ProtoReflect()
	unknown := stripField(r.GetUnknown(), num)
	if userID > 0 {
		unknown = protowire.AppendTag(unknown, num, protowire.VarintType)
		unknown = protowire.AppendVarint(unknown, uint64(userID))
	}
	r.SetUnknown(unknown)
}

func userField(msg interface{ ProtoReflect() protoreflect.Message }, num protowire.Number) int64 {
	v, ok := readVarintField(msg.ProtoReflect().GetUnknown(), num)
	if !ok || v == 0 {
		return 0
	}
	return int64(v)
}

func stripField(b []byte, drop protowire.Number) []byte {
	var out []byte
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return append(out, b...)
		}
		rest := b[n:]
		vlen := protowire.ConsumeFieldValue(num, typ, rest)
		if vlen < 0 {
			return append(out, b...)
		}
		if num != drop {
			out = append(out, b[:n+vlen]...)
		}
		b = rest[vlen:]
	}
	return out
}

func readVarintField(b []byte, want protowire.Number) (uint64, bool) {
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return 0, false
		}
		rest := b[n:]
		vlen := protowire.ConsumeFieldValue(num, typ, rest)
		if vlen < 0 {
			return 0, false
		}
		if num == want && typ == protowire.VarintType {
			v, m := protowire.ConsumeVarint(rest)
			if m < 0 {
				return 0, false
			}
			return v, true
		}
		b = rest[vlen:]
	}
	return 0, false
}
