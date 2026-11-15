package core

import "github.com/teamgram/proto/mtproto"

func chatsMatching(chats []*mtproto.Chat, ids []int64) []*mtproto.Chat {
	if len(chats) == 0 || len(ids) == 0 {
		return []*mtproto.Chat{}
	}
	want := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		want[id] = struct{}{}
	}
	out := make([]*mtproto.Chat, 0, len(ids))
	for _, ch := range chats {
		if ch == nil {
			continue
		}
		if _, ok := want[ch.GetId()]; ok {
			out = append(out, ch)
		}
	}
	return out
}
