package channel

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestMapDraftWriteError(t *testing.T) {
	if mapDraftWriteError(false, false, nil) != nil {
		t.Fatal("allowed")
	}
	if mapDraftWriteError(false, false, mtproto.ErrChannelPrivate) != mtproto.ErrChatWriteForbidden {
		t.Fatal("non-member")
	}
	if mapDraftWriteError(true, false, mtproto.ErrChannelPrivate) != mtproto.ErrChatWriteForbidden {
		t.Fatal("broadcast non-member")
	}
	if mapDraftWriteError(true, false, mtproto.ErrChatWriteForbidden) != mtproto.ErrChatAdminRequired {
		t.Fatal("broadcast needs an admin")
	}
	if mapDraftWriteError(true, true, mtproto.ErrChatWriteForbidden) != mtproto.ErrChatWriteForbidden {
		t.Fatal("muted broadcast member")
	}
	if mapDraftWriteError(false, true, mtproto.ErrChatWriteForbidden) != mtproto.ErrChatWriteForbidden {
		t.Fatal("muted megagroup member")
	}
	if mapDraftWriteError(false, false, mtproto.ErrUserBannedInChannel) != mtproto.ErrUserBannedInChannel {
		t.Fatal("view ban")
	}
}
