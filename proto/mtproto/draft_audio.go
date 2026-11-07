package mtproto

// DraftTranscribeConstructor is messages.transcribeDraft#7e11da47.
// It is a Tellomi method on the same messages.transcribedAudio result as
// messages.transcribeAudio. The audio bytes are not a stored message.
// See docs/api/draft-transcribe.md.
const DraftTranscribeConstructor TLConstructor = 0x7e11da47

const draftAudioField = 15

// SetDraftAudio marks this request as an unsent recording. The bytes travel
// with the RPC and are not a document in storage.
func (m *TLMessagesTranscribeAudio) SetDraftAudio(b []byte) {
	if m == nil {
		return
	}
	m.Constructor = DraftTranscribeConstructor
	if len(b) == 0 {
		m.ProtoReflect().SetUnknown(nil)
		return
	}
	m.ProtoReflect().SetUnknown(appendProtoBytes(draftAudioField, b))
}

// DraftAudio returns the unsent recording, or nil for a normal transcribeAudio.
func (m *TLMessagesTranscribeAudio) DraftAudio() []byte {
	if m == nil {
		return nil
	}
	raw := m.ProtoReflect().GetUnknown()
	if m.Constructor != DraftTranscribeConstructor && len(raw) == 0 {
		return nil
	}
	return readProtoBytes(draftAudioField, raw)
}

// ClearDraftAudio drops the recording from this request object.
func (m *TLMessagesTranscribeAudio) ClearDraftAudio() {
	if m == nil {
		return
	}
	m.ProtoReflect().SetUnknown(nil)
}

func appendProtoBytes(field int, b []byte) []byte {
	tag := uint64(field<<3 | 2)
	out := appendUvarint(nil, tag)
	out = appendUvarint(out, uint64(len(b)))
	return append(out, b...)
}

func readProtoBytes(field int, raw []byte) []byte {
	want := uint64(field<<3 | 2)
	for len(raw) > 0 {
		tag, n := uvarint(raw)
		if n <= 0 {
			return nil
		}
		raw = raw[n:]
		wire := tag & 7
		if wire != 2 {
			return nil
		}
		size, n := uvarint(raw)
		if n <= 0 || uint64(len(raw)-n) < size {
			return nil
		}
		raw = raw[n:]
		val := raw[:size]
		raw = raw[size:]
		if tag == want {
			out := make([]byte, len(val))
			copy(out, val)
			return out
		}
	}
	return nil
}

func appendUvarint(dst []byte, v uint64) []byte {
	for v >= 0x80 {
		dst = append(dst, byte(v)|0x80)
		v >>= 7
	}
	return append(dst, byte(v))
}

func uvarint(b []byte) (uint64, int) {
	var x uint64
	for i, c := range b {
		if i > 9 {
			return 0, -1
		}
		if c < 0x80 {
			return x | uint64(c)<<uint(7*i), i + 1
		}
		x |= uint64(c&0x7f) << uint(7*i)
	}
	return 0, 0
}
