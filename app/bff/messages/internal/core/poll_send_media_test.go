package core

import (
	"bytes"
	"context"
	"encoding/binary"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/zeromicro/go-zero/core/logx"
)

// TestSendMediaBuildsLayer232Poll is the messages.sendMedia media step for
// ISSUE-S-POLL-001 row 70. TELLOMI_POLLS=1 is set only in this process.
func TestSendMediaBuildsLayer232Poll(t *testing.T) {
	t.Setenv("TELLOMI_POLLS", "1")
	chatbox.ResetShared()
	t.Cleanup(chatbox.ResetShared)

	c := &MessagesCore{
		ctx:    context.Background(),
		Logger: logx.WithContext(context.Background()),
		MD:     &metadata.RpcMetadata{UserId: 1},
	}
	media, err := c.makeMediaByInputMedia(mtproto.MakeTLInputMediaPoll(&mtproto.InputMedia{
		Poll: mtproto.MakeTLPoll(&mtproto.Poll{
			PublicVoters: true,
			Question:     "fixture-q",
			Answers: []*mtproto.PollAnswer{
				mtproto.MakePollAnswerWithText("yes", []byte("0")),
				mtproto.MakePollAnswerWithText("no", []byte("1")),
			},
		}).To_Poll(),
	}).To_InputMedia())
	if err != nil {
		t.Fatal(err)
	}
	if media.GetPredicateName() != mtproto.Predicate_messageMediaPoll {
		t.Fatalf("media %s", media.GetPredicateName())
	}
	if media.GetPoll().GetQuestion_TEXTWITHENTITIES().GetText() != "fixture-q" || !media.GetPoll().GetPublicVoters() {
		t.Fatalf("poll %+v", media.GetPoll())
	}

	msg := mtproto.MakeTLMessage(&mtproto.Message{
		Out:     true,
		Id:      11,
		FromId:  mtproto.MakePeerUser(1),
		PeerId:  mtproto.MakePeerUser(2),
		Date:    1_700_000_000,
		Message: "",
		Media:   media,
	}).To_Message()
	buf := mtproto.NewEncodeBuf(256)
	if err = msg.Encode(buf, mtproto.Layer); err != nil {
		t.Fatal(err)
	}
	raw := buf.GetBuf()
	var pollCtor, unsupported [4]byte
	binary.LittleEndian.PutUint32(pollCtor[:], 0x773f4e66)
	binary.LittleEndian.PutUint32(unsupported[:], 0x9f84f49e)
	if !bytes.Contains(raw, pollCtor[:]) || bytes.Contains(raw, unsupported[:]) {
		t.Fatal("sendMedia poll did not encode as messageMediaPoll#773f4e66")
	}
}
