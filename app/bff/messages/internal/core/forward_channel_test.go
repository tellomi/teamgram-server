package core

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestBoxesFromChannelMessagesSkipsEmpty(t *testing.T) {
	live := mtproto.MakeTLMessage(&mtproto.Message{Id: 4, Message: "hi", FromId: mtproto.MakePeerUser(7)}).To_Message()
	empty := mtproto.MakeTLMessageEmpty(&mtproto.Message{Id: 5}).To_Message()
	got := boxesFromChannelMessages(99, []*mtproto.Message{empty, live, nil})
	if got.Length() != 1 || got.Datas[0].SenderUserId != 7 || got.Datas[0].PeerId != 99 || got.Datas[0].MessageId != 4 {
		t.Fatalf("%#v", got.Datas)
	}
}
