package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSearchCountersVoiceAndRoundAreImmediate covers
// ISSUE-S-SEARCHCOUNT-VOICE-LAG-001 (row 360). A photo and a voice note
// sent one after the other used to answer photo=1, voice=0 from the same
// messages.getSearchCounters call. The count has to be right on the next
// call, with no wait.
//
// Phones 9996610811 and 9996610812, code 11111.
func TestSearchCountersVoiceAndRoundAreImmediate(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()

	a, _ := dialPhone(t, ctx, cfg, "9996610811", "LagA")
	_, _ = dialPhone(t, ctx, cfg, "9996610812", "LagB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610812", "LagB")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(bID, bHash)
	sendCountedMedia(t, a, peer)
	assertImmediateCounters(t, a, peer)

	chID, chHash, err := a.CreateMegagroup(ctx, "lag-mega")
	if err != nil {
		t.Fatal(err)
	}
	sendCountedMedia(t, a, inputPeerChannel(chID, chHash))
	assertImmediateCounters(t, a, inputPeerChannel(chID, chHash))
}

func sendCountedMedia(t *testing.T, c *Client, peer *mtproto.InputPeer) {
	t.Helper()
	sendPhotoBytes(t, c, peer, "shot.jpg", testJPEG(t, 320, 240, 5))
	sendDoc(t, c, peer, "audio/ogg", []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true, Duration: 1}).To_DocumentAttribute(),
	})
	sendDoc(t, c, peer, "video/mp4", []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
			RoundMessage: true, W: 240, H: 240, Duration: 1,
		}).To_DocumentAttribute(),
	})
}

func assertImmediateCounters(t *testing.T, c *Client, peer *mtproto.InputPeer) {
	t.Helper()
	photos := mtproto.MakeTLInputMessagesFilterPhotos(&mtproto.MessagesFilter{}).To_MessagesFilter()
	voice := mtproto.MakeTLInputMessagesFilterVoice(&mtproto.MessagesFilter{}).To_MessagesFilter()
	round := mtproto.MakeTLInputMessagesFilterRoundVideo(&mtproto.MessagesFilter{}).To_MessagesFilter()
	pair := mtproto.MakeTLInputMessagesFilterRoundVoice(&mtproto.MessagesFilter{}).To_MessagesFilter()
	res := invokeOK(t, c, &mtproto.TLMessagesGetSearchCounters{
		Constructor: mtproto.CRC32_messages_getSearchCounters_732eef00,
		Peer:        peer,
		Filters:     []*mtproto.MessagesFilter{photos, voice, round, pair},
	})
	var rows []*mtproto.Messages_SearchCounter
	switch vec := res.(type) {
	case *mtproto.Vector_Messages_SearchCounter:
		rows = vec.GetDatas()
	case *tlVector:
		rows = vec.Counters
	default:
		t.Fatalf("messages.getSearchCounters returned %T", res)
	}
	got := map[mtproto.MessagesFilterType]int32{}
	for _, row := range rows {
		if row.GetInexact() {
			t.Fatalf("counter is inexact: %s", row)
		}
		got[mtproto.FromMessagesFilter(row.GetFilter())] = row.GetCount()
	}
	want := map[mtproto.MessagesFilterType]int32{
		mtproto.FilterPhotos:     1,
		mtproto.FilterVoice:      1,
		mtproto.FilterRoundVideo: 1,
		mtproto.FilterRoundVoice: 2,
	}
	for kind, n := range want {
		if got[kind] != n {
			t.Fatalf("counters %+v, want %+v", got, want)
		}
	}
}
