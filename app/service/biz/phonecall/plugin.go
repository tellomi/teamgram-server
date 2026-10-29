package phonecall

import (
	"context"
	"sync"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/chat/plugin"
)

// Open opens MySQL once for the chat plugin. A failure leaves the plugin
// returning no call, which is the same as a chat with no voice chat.
var openOnce sync.Once

type chatPlugin struct {
	dsn string
}

// NewChatPlugin fills chat.call / call_active from phone_peer_call.
func NewChatPlugin(dsn string) plugin.ChatPlugin {
	return &chatPlugin{dsn: dsn}
}

func (p *chatPlugin) ensure() {
	openOnce.Do(func() { _ = UseDSN(p.dsn) })
}

func (p *chatPlugin) GetChatGroupCall(ctx context.Context, userId int64, chatId int64) *mtproto.InputGroupCall {
	p.ensure()
	id, access, active, _, err := LookupDB(ctx, p.dsn, mtproto.PEER_CHAT, chatId)
	if err != nil || !active || id == 0 {
		return nil
	}
	return mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: id, AccessHash: access}).To_InputGroupCall()
}

func (p *chatPlugin) GetChatCallActiveAndNotEmpty(ctx context.Context, userId int64, chatId int64) (bool, bool) {
	p.ensure()
	_, _, active, notEmpty, err := LookupDB(ctx, p.dsn, mtproto.PEER_CHAT, chatId)
	if err != nil {
		return false, false
	}
	return active, notEmpty
}
