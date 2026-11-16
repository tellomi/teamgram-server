package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestSearchCountersVoiceAndRoundConverge covers
// ISSUE-S-SEARCHCOUNT-VOICE-LAG-001 (row 360).
//
// The sender's messages row is inserted by the inbox consumer after
// sendMedia returns. A photo sent first can already be counted while the
// voice note is still in Kafka. A photo sent last lags the same way.
// getSearchCounters counts both from message_filter_type with count(id);
// it does not wait inside the RPC. This test polls that RPC until the
// four counts match, and stops at a deadline. It does not sleep for a
// fixed time.
//
// Phones 9996610831 and 9996610832, code 11111.
func TestSearchCountersVoiceAndRoundConverge(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := contextTimeout(90 * time.Second)
	defer cancel()

	a, _ := dialPhone(t, ctx, cfg, "9996610831", "LagA")
	_, _ = dialPhone(t, ctx, cfg, "9996610832", "LagB")
	bID, bHash, err := a.ImportPhone(ctx, "9996610832", "LagB")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(bID, bHash)
	sendCountedMedia(t, a, peer)
	waitSearchCounters(t, a, peer)

	chID, chHash, err := a.CreateMegagroup(ctx, "lag-mega")
	if err != nil {
		t.Fatal(err)
	}
	sendCountedMedia(t, a, inputPeerChannel(chID, chHash))
	waitSearchCounters(t, a, inputPeerChannel(chID, chHash))
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

func waitSearchCounters(t *testing.T, c *Client, peer *mtproto.InputPeer) {
	t.Helper()
	ctx, cancel := contextTimeout(15 * time.Second)
	defer cancel()
	var last map[mtproto.MessagesFilterType]int32
	err := waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		last = readSearchCounters(t, c, peer)
		return countersConverged(last), nil
	})
	if err != nil {
		t.Fatalf("counters did not converge: %+v: %v", last, err)
	}
}

func countersConverged(got map[mtproto.MessagesFilterType]int32) bool {
	return got[mtproto.FilterPhotos] == 1 &&
		got[mtproto.FilterVoice] == 1 &&
		got[mtproto.FilterRoundVideo] == 1 &&
		got[mtproto.FilterRoundVoice] == 2
}

func readSearchCounters(t *testing.T, c *Client, peer *mtproto.InputPeer) map[mtproto.MessagesFilterType]int32 {
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
	return got
}
