package mtproto

import "testing"

func TestFromBoolUsesConstructor(t *testing.T) {
	if !FromBool(&Bool{Constructor: CRC32_boolTrue}) {
		t.Fatal("boolTrue constructor")
	}
	if FromBool(&Bool{Constructor: CRC32_boolFalse}) {
		t.Fatal("boolFalse constructor")
	}
	if !FromBool(BoolTrue) || FromBool(BoolFalse) || FromBool(nil) {
		t.Fatal("named bools")
	}
}

func TestFromMessagesFilterUsesConstructor(t *testing.T) {
	got := FromMessagesFilter(&MessagesFilter{Constructor: CRC32_inputMessagesFilterVoice})
	if got != FilterVoice {
		t.Fatalf("voice constructor = %d", got)
	}
	got = FromMessagesFilter(&MessagesFilter{Constructor: CRC32_inputMessagesFilterUrl})
	if got != FilterUrl {
		t.Fatalf("url constructor = %d", got)
	}
	if FromMessagesFilter(nil) != FilterEmpty {
		t.Fatal("nil filter")
	}
	buf := NewEncodeBuf(32)
	only := &MessagesFilter{Constructor: CRC32_inputMessagesFilterPhotos}
	if err := only.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
}
