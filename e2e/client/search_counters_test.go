package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSearchCountersPhotoVoiceAndMegagroup covers ISSUE-S-SEARCH-COUNTERS-001
// (row 75) and ISSUE-S-SEARCHCOUNT-001 (row 120). A private photo used to
// count as 0 because the counter looked up MEDIA_PHOTOVIDEO while the row
// is stored as MEDIA_PHOTOS_ONLY. Voice was unmapped. A megagroup counted
// the private box, which never holds channel posts.
func TestSearchCountersPhotoVoiceAndMegagroup(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()

	a, _ := dialPhone(t, ctx, cfg, "9996610441", "CntA")
	_, _ = dialPhone(t, ctx, cfg, "9996610442", "CntB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610442", "CntB")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(bID, bHash)

	if _, err = a.SendText(ctx, peer, "hello https://tellomi.cn/counters", []*mtproto.MessageEntity{
		mtproto.MakeTLMessageEntityUrl(&mtproto.MessageEntity{Offset: 6, Length: 27}).To_MessageEntity(),
	}); err != nil {
		t.Fatal(err)
	}
	sendPhotoBytes(t, a, peer, "shot.jpg", testJPEG(t, 320, 240, 3))
	sendDoc(t, a, peer, "audio/ogg", []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true, Duration: 1}).To_DocumentAttribute(),
	})

	photos := mtproto.MakeTLInputMessagesFilterPhotos(&mtproto.MessagesFilter{}).To_MessagesFilter()
	videos := mtproto.MakeTLInputMessagesFilterVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()
	both := mtproto.MakeTLInputMessagesFilterPhotoVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()
	voice := mtproto.MakeTLInputMessagesFilterVoice(&mtproto.MessagesFilter{}).To_MessagesFilter()
	round := mtproto.MakeTLInputMessagesFilterRoundVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()
	urls := mtproto.MakeTLInputMessagesFilterUrl(&mtproto.MessagesFilter{}).To_MessagesFilter()

	got := searchCounterMap(t, a, peer, photos, videos, both, voice, round, urls)
	if got[mtproto.FilterPhotos] < 1 || got[mtproto.FilterPhotoVideo] < 1 || got[mtproto.FilterVideo] != 0 {
		t.Fatalf("private photo counters %+v", got)
	}
	if got[mtproto.FilterVoice] < 1 || got[mtproto.FilterRoundVideo] != 0 {
		t.Fatalf("private voice counters %+v", got)
	}
	if got[mtproto.FilterUrl] < 1 {
		t.Fatalf("private url counter %+v", got)
	}
	found := searchPeer(t, a, peer, photos)
	if len(found) < 1 {
		t.Fatal("private messages.search photos returned nothing")
	}

	chID, chHash, err := a.CreateMegagroup(ctx, "cnt-mega")
	if err != nil {
		t.Fatal(err)
	}
	mega := inputPeerChannel(chID, chHash)
	sendPhotoBytes(t, a, mega, "group.jpg", testJPEG(t, 320, 240, 4))
	megaGot := searchCounterMap(t, a, mega, photos, videos, both, voice, round)
	if megaGot[mtproto.FilterPhotos] < 1 || megaGot[mtproto.FilterPhotoVideo] < 1 || megaGot[mtproto.FilterVideo] != 0 || megaGot[mtproto.FilterVoice] != 0 {
		t.Fatalf("megagroup counters %+v", megaGot)
	}
	if len(searchPeer(t, a, mega, photos)) < 1 {
		t.Fatal("megagroup messages.search photos returned nothing")
	}
}

func searchCounterMap(t *testing.T, c *Client, peer *mtproto.InputPeer, filters ...*mtproto.MessagesFilter) map[mtproto.MessagesFilterType]int32 {
	t.Helper()
	res := invokeOK(t, c, &mtproto.TLMessagesGetSearchCounters{
		Constructor: mtproto.CRC32_messages_getSearchCounters_732eef00,
		Peer:        peer,
		Filters:     filters,
	})
	out := map[mtproto.MessagesFilterType]int32{}
	switch vec := res.(type) {
	case *mtproto.Vector_Messages_SearchCounter:
		for _, row := range vec.GetDatas() {
			out[mtproto.FromMessagesFilter(row.GetFilter())] = row.GetCount()
		}
	case *tlVector:
		// layer 232 returns a bare Vector<messages.SearchCounter>.
		for _, row := range vec.Counters {
			out[mtproto.FromMessagesFilter(row.GetFilter())] = row.GetCount()
		}
	default:
		t.Fatalf("messages.getSearchCounters returned %T", res)
	}
	return out
}

func searchPeer(t *testing.T, c *Client, peer *mtproto.InputPeer, filter *mtproto.MessagesFilter) []*mtproto.Message {
	t.Helper()
	res := invokeOK(t, c, &mtproto.TLMessagesSearch{
		Constructor: mtproto.CRC32_messages_search_a0fda762,
		Peer:        peer,
		Filter:      filter,
		Limit:       20,
	})
	msgs, ok := messagesOf(res)
	if !ok {
		t.Fatalf("messages.search returned %T", res)
	}
	return msgs
}
