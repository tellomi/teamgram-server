package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestDocumentByHashSize(t *testing.T) {
	old := &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_338e2464,
		Size2_INT32: 42,
		Size2_INT64: 99,
	}
	if got := documentByHashSize(old); got != 42 {
		t.Fatalf("old constructor size %d", got)
	}
	cur := &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Size2_INT64: 1000,
		Size2_INT32: 1,
	}
	if got := documentByHashSize(cur); got != 1000 {
		t.Fatalf("current constructor size %d", got)
	}
	fallback := &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Size2_INT32: 7,
	}
	if got := documentByHashSize(fallback); got != 7 {
		t.Fatalf("fallback size %d", got)
	}
}
