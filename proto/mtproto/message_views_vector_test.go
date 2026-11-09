package mtproto

import (
	"testing"

	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Layer 232 message puts views and forwards behind the same flag bit.
// A broadcast post stores views only. Both integers have to be written or
// the decoder consumes the next vector and the chats list disappears.
func TestViewsAndForwardsShareAFlag(t *testing.T) {
	msg := MakeTLMessage(&Message{
		Id:      2,
		Post:    true,
		Message: "上海你好 #新闻",
		Date:    10,
		PeerId:  MakePeerChannel(1073800005),
		Views:   wrapperspb.Int32(1),
	}).To_Message()
	res := MakeTLMessagesMessagesSlice(&Messages_Messages{
		Count:    1,
		Messages: []*Message{msg},
		Topics:   []*ForumTopic{},
		Chats: []*Chat{MakeTLChannel(&Chat{
			Broadcast: true,
			Id:        1073800005,
			Title:     "中文新闻台",
			Photo:     MakeTLChatPhotoEmpty(nil).To_ChatPhoto(),
		}).To_Chat()},
		Users: []*User{MakeTLUser(&User{Id: 7, Self: true}).To_User()},
	}).To_Messages_Messages()

	buf := NewEncodeBuf(1024)
	if err := res.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	d := NewDecodeBuf(buf.GetBuf())
	obj := d.Object()
	if err := d.GetError(); err != nil {
		t.Fatal(err)
	}
	slice, ok := obj.(*TLMessagesMessagesSlice)
	if !ok {
		t.Fatalf("result %T", obj)
	}
	got := slice.To_Messages_Messages()
	if len(got.GetMessages()) != 1 || got.GetMessages()[0].GetMessage() != "上海你好 #新闻" {
		t.Fatalf("message %#v", got.GetMessages())
	}
	if got.GetMessages()[0].GetViews().GetValue() != 1 {
		t.Fatalf("views %#v", got.GetMessages()[0].GetViews())
	}
	if len(got.GetChats()) != 1 || got.GetChats()[0].GetId() != 1073800005 {
		t.Fatalf("chats %#v", got.GetChats())
	}
	if len(got.GetUsers()) != 1 || got.GetUsers()[0].GetId() != 7 {
		t.Fatalf("users %#v", got.GetUsers())
	}
}
