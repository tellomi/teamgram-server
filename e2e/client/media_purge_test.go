package e2e

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMediaPurgeCopies checks delete-for-me against delete-for-everyone.
// A sends a photo, a video, and a voice note to B. B forwards each to C.
// A's delete-for-me leaves B and C able to download. After every copy is
// gone, the purge queue holds the ids and, once the worker runs, getFile
// fails.
func TestMediaPurgeCopies(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "purge_a", "PurgeA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "purge_b", "PurgeB")
	c, _ := loginNamed(t, cfg, ProfileAndroid, "purge_c", "PurgeC")
	aToB := userPeer(t, cfg, a, "purge_b", "PurgeB")
	bToA := userPeer(t, cfg, b, "purge_a", "PurgeA")
	bToC := userPeer(t, cfg, b, "purge_c", "PurgeC")
	cToB := userPeer(t, cfg, c, "purge_b", "PurgeB")

	photoBytes := testJPEG(t, 320, 240, 0x5a)
	photo, photoMsg := sendPhotoBytes(t, a, aToB, "purge.jpg", photoBytes)
	videoPayload := []byte("tellomi-purge-video-copies-v1")
	video, videoMsg := sendPayload(t, a, aToB, "video/mp4", "purge.mp4", videoPayload)
	voicePayload := []byte("tellomi-purge-voice-copies-v1")
	voice, voiceMsg := sendVoiceBytes(t, a, aToB, "purge-voice", voicePayload)
	videoKeys := storedObjects(t, documentObjectKeys(t, video.GetId()))
	if len(videoKeys) == 0 {
		t.Fatal("video upload did not store an object")
	}

	photoFwd := forwardReturning(t, b, bToA, func(m *mtproto.Message) bool {
		return m.GetMedia().GetPhoto_FLAGPHOTO().GetId() == photo.GetId()
	}, bToC)
	videoFwd := forwardReturning(t, b, bToA, func(m *mtproto.Message) bool {
		return m.GetMedia().GetDocument().GetId() == video.GetId()
	}, bToC)
	voiceFwd := forwardReturning(t, b, bToA, func(m *mtproto.Message) bool {
		return m.GetMedia().GetDocument().GetId() == voice.GetId()
	}, bToC)
	_ = photoFwd
	_ = videoFwd
	_ = voiceFwd

	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      false,
		Id:          []int32{photoMsg, videoMsg, voiceMsg},
	})
	if body := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT CONCAT(message, '|', message_data) FROM messages WHERE user_id=%d AND user_message_box_id=%d`, aSelf.ID, photoMsg))); body != "|{}" && body != "|null" {
		t.Fatalf("delete-for-me left body %q", body)
	}
	if peerBody := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT message_data FROM messages WHERE user_id=%d AND user_message_box_id=%d`, bSelf.ID, findBox(t, b, bToA, func(m *mtproto.Message) bool {
			return m.GetMedia().GetPhoto_FLAGPHOTO().GetId() == photo.GetId()
		})))); !strings.Contains(peerBody, fmt.Sprintf("%d", photo.GetId())) {
		t.Fatalf("delete-for-me wiped B's copy: %s", peerBody)
	}
	size := fullPhotoSize(t, photo)
	if len(waitNonEmpty(t, b, photoLocation(photo, size))) == 0 || len(waitNonEmpty(t, c, photoLocation(photo, size))) == 0 {
		t.Fatal("photo download failed while B or C still has a copy")
	}
	if !bytes.Equal(waitFile(t, b, docLocation(video), videoPayload), videoPayload) {
		t.Fatal("video download failed for B")
	}
	if !bytes.Equal(waitFile(t, c, docLocation(voice), voicePayload), voicePayload) {
		t.Fatal("voice download failed for C")
	}

	bPhoto := findBox(t, b, bToA, func(m *mtproto.Message) bool {
		return m.GetMedia().GetPhoto_FLAGPHOTO().GetId() == photo.GetId()
	})
	bVideo := findBox(t, b, bToA, func(m *mtproto.Message) bool {
		return m.GetMedia().GetDocument().GetId() == video.GetId()
	})
	bVoice := findBox(t, b, bToA, func(m *mtproto.Message) bool {
		return m.GetMedia().GetDocument().GetId() == voice.GetId()
	})
	invokeOK(t, b, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{bPhoto, bVideo, bVoice},
	})
	cPhoto := findBox(t, c, cToB, func(m *mtproto.Message) bool {
		return m.GetMedia().GetPhoto_FLAGPHOTO().GetId() == photo.GetId()
	})
	cVideo := findBox(t, c, cToB, func(m *mtproto.Message) bool {
		return m.GetMedia().GetDocument().GetId() == video.GetId()
	})
	cVoice := findBox(t, c, cToB, func(m *mtproto.Message) bool {
		return m.GetMedia().GetDocument().GetId() == voice.GetId()
	})
	invokeOK(t, c, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{cPhoto, cVideo, cVoice},
	})

	// Revoke returns after the caller's rows are deleted. The other side is
	// deleted by the inbox consumer, and that is what writes the purge row
	// once no copy remains. An immediate count races that Kafka handoff.
	for _, pair := range []struct {
		kind string
		id   int64
	}{{"photo", photo.GetId()}, {"document", video.GetId()}, {"document", voice.GetId()}} {
		n := waitPurgeQueue(t, pair.kind, pair.id)
		if n != "1" {
			t.Fatalf("queue %s %d count %s, want 1", pair.kind, pair.id, n)
		}
	}
	runPurge(t)
	requireObjectsDeleted(t, videoKeys)
	// upload.getFile caches "not expired" for a minute after the downloads above.
	time.Sleep(65 * time.Second)
	expectFileGone(t, a, photoLocation(photo, size))
	expectFileGone(t, b, docLocation(video))
	expectFileGone(t, c, docLocation(voice))
	if n := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*) FROM documents WHERE document_id IN (%d,%d) AND expired_at=0`, video.GetId(), voice.GetId()))); n != "0" {
		t.Fatalf("documents still downloadable: %s not expired", n)
	}
}

// TestMediaPurgeHistory covers revoke and just_clear. Revoke wipes both
// boxes. just_clear wipes only the caller's boxes; the other side keeps
// the file.
func TestMediaPurgeHistory(t *testing.T) {
	cfg := requireE2E(t)
	a, aSelf := loginNamed(t, cfg, ProfileIOS, "purge_hist_a", "HistA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "purge_hist_b", "HistB")
	aToB := userPeer(t, cfg, a, "purge_hist_b", "HistB")
	bToA := userPeer(t, cfg, b, "purge_hist_a", "HistA")

	secret := "purge-history-secret"
	cx, cancel := ctx()
	if _, err := a.SendText(cx, aToB, secret, nil); err != nil {
		cancel()
		t.Fatal(err)
	}
	cancel()
	payload := []byte("tellomi-purge-history-file")
	doc, _ := sendPayload(t, a, aToB, "application/octet-stream", "hist.bin", payload)
	bID := findBox(t, b, bToA, func(m *mtproto.Message) bool {
		return m.GetMedia().GetDocument().GetId() == doc.GetId() || m.GetMessage() == secret
	})
	if bID == 0 {
		t.Fatal("B did not receive the history messages")
	}
	invokeOK(t, a, &mtproto.TLMessagesDeleteHistory{
		Constructor: mtproto.CRC32_messages_deleteHistory,
		Peer:        aToB,
		Revoke:      true,
	})
	if got := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*) FROM messages WHERE user_id IN (%d,%d) AND deleted=0 AND message='%s'`, aSelf.ID, bSelf.ID, secret))); got != "0" {
		t.Fatalf("revoke left %s live rows containing the secret", got)
	}
	if got := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*) FROM messages WHERE user_id IN (%d,%d) AND message LIKE '%%%s%%'`, aSelf.ID, bSelf.ID, secret))); got != "0" {
		t.Fatalf("wiped rows still store the secret (%s)", got)
	}

	again := []byte("tellomi-purge-history-keep")
	keep, _ := sendPayload(t, a, aToB, "application/octet-stream", "keep.bin", again)
	invokeOK(t, a, &mtproto.TLMessagesDeleteHistory{
		Constructor: mtproto.CRC32_messages_deleteHistory,
		Peer:        aToB,
		JustClear:   true,
	})
	if !bytes.Equal(waitFile(t, b, docLocation(keep), again), again) {
		t.Fatal("just_clear removed B's copy")
	}
	_ = doc
}

// TestMediaPurgeDedupRefCount re-uploads the same bytes from two accounts.
// Deleting one account's messages leaves ref_count at 1 and the other
// download working. Deleting the rest drops the blob.
func TestMediaPurgeDedupRefCount(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "purge_dedup_a", "DedupPA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "purge_dedup_b", "DedupPB")
	aToB := userPeer(t, cfg, a, "purge_dedup_b", "DedupPB")
	bToA := userPeer(t, cfg, b, "purge_dedup_a", "DedupPA")
	payload := []byte("tellomi-purge-dedup-same-bytes-v1")
	docA, msgA := sendPayload(t, a, aToB, "application/octet-stream", "same.bin", payload)
	docB, msgB := sendPayload(t, b, bToA, "application/octet-stream", "same.bin", payload)
	if docA.GetId() == docB.GetId() {
		t.Fatal("re-upload reused the document id")
	}
	// The shared blob is one object. Capture it before either delete releases
	// the ref row the path is read from.
	shared := storedObjects(t, append(documentObjectKeys(t, docA.GetId()), documentObjectKeys(t, docB.GetId())...))
	if len(shared) == 0 {
		t.Fatal("dedup upload did not store an object")
	}
	before := blobRefCount(t, docA.GetId())
	if before != "2" {
		t.Fatalf("ref_count %s, want 2", before)
	}
	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{msgA},
	})
	runPurge(t)
	if got := blobRefCount(t, docB.GetId()); got != "1" {
		t.Fatalf("ref_count after one delete %s, want 1", got)
	}
	if !bytes.Equal(waitFile(t, b, docLocation(docB), payload), payload) {
		t.Fatal("shared object deleted while B's message remains")
	}
	requireObjectsStored(t, shared)
	invokeOK(t, b, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{msgB},
	})
	runPurge(t)
	requireObjectsDeleted(t, shared)
	if got := blobRefCount(t, docB.GetId()); got != "0" && got != "" {
		t.Fatalf("ref_count after both deletes %s, want 0", got)
	}
	// B downloaded docB before this purge, which caches "not expired" for a minute.
	time.Sleep(65 * time.Second)
	expectFileGone(t, a, docLocation(docA))
	expectFileGone(t, b, docLocation(docB))
}

// TestMediaPurgeChannelDelete wipes a channel post. Auto-delete TTL uses
// the same CommitDelete when TELLOMI_MESSAGE_EXPIRY_SWEEP=1; this test
// calls channels.deleteMessages, which is that path, and also reads history
// after the channel ttl is already in the past.
func TestMediaPurgeChannelDelete(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "purge_ch_a", "ChA")
	cx, cancel := ctx()
	id, hash, err := a.CreateMegagroup(cx, "purge-channel")
	cancel()
	if err != nil {
		t.Fatal(err)
	}
	ch := inputChannel(id, hash)
	peer := inputPeerChannel(id, hash)
	secret := "purge-channel-secret"
	sendCtx, sendCancel := ctx()
	msgID, err := a.SendText(sendCtx, peer, secret, nil)
	sendCancel()
	if err != nil {
		t.Fatal(err)
	}
	if msgID == 0 {
		t.Fatal("channel send returned no message")
	}
	mustSQL(t, fmt.Sprintf(`UPDATE channels SET ttl_period=1 WHERE id=%d`, id))
	mustSQL(t, fmt.Sprintf(`UPDATE channel_messages SET date2=1 WHERE channel_id=%d AND message_id=%d`, id, msgID))
	invokeOK(t, a, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        peer,
		Limit:       20,
	})
	body := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT CONCAT(deleted, ':', IFNULL(message,''), ':', OCTET_LENGTH(message_data)) FROM channel_messages WHERE channel_id=%d AND message_id=%d`, id, msgID)))
	if !strings.HasPrefix(body, "1:") {
		invokeOK(t, a, &mtproto.TLChannelsDeleteMessages{
			Constructor: mtproto.CRC32_channels_deleteMessages,
			Channel:     ch,
			Id:          []int32{msgID},
		})
		body = strings.TrimSpace(mustSQL(t, fmt.Sprintf(
			`SELECT CONCAT(deleted, ':', IFNULL(message,''), ':', OCTET_LENGTH(message_data)) FROM channel_messages WHERE channel_id=%d AND message_id=%d`, id, msgID)))
	}
	if body != "1::0" && body != "1::2" {
		t.Fatalf("channel body %q, want deleted with empty text", body)
	}
	if strings.Contains(body, secret) {
		t.Fatalf("channel row still contains the secret: %s", body)
	}
}

func fullPhotoSize(t *testing.T, p *mtproto.Photo) string {
	t.Helper()
	have := map[string]bool{}
	for _, sz := range p.GetSizes() {
		if sz.GetType() != "" {
			have[sz.GetType()] = true
		}
	}
	for _, want := range []string{"y", "x", "w", "m", "0"} {
		if have[want] {
			return want
		}
	}
	return largestPhotoSize(t, p)
}

// TestMediaPurgeLegacyKeepsObject strips the blob ref from an uploaded
// document so it looks like a pre-dedup row (file_path only). Deleting the
// message must not remove that object.
func TestMediaPurgeLegacyKeepsObject(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "purge_a", "PurgeA")
	aToB := userPeer(t, cfg, a, "purge_b", "PurgeB")
	payload := []byte("tellomi-purge-legacy-file-v1")
	doc, msg := sendPayload(t, a, aToB, "application/octet-stream", "legacy.bin", payload)
	path := strings.TrimSpace(mustSQL(t, fmt.Sprintf(`SELECT file_path FROM documents WHERE document_id=%d`, doc.GetId())))
	if path != fmt.Sprintf("%d.dat", doc.GetId()) {
		t.Fatalf("origin file_path %q", path)
	}
	mustSQL(t, fmt.Sprintf(`DELETE FROM media_blob_refs WHERE ref_kind='document' AND ref_id=%d`, doc.GetId()))
	mustSQL(t, fmt.Sprintf(`DELETE FROM media_blobs WHERE file_path='%s' AND storage_class='documents'`, path))
	mustSQL(t, fmt.Sprintf(`UPDATE documents SET blob_id=0 WHERE document_id=%d`, doc.GetId()))
	if n := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*) FROM media_blob_refs WHERE ref_kind='document' AND ref_id=%d`, doc.GetId()))); n != "0" {
		t.Fatalf("blob ref remains: %s", n)
	}
	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{msg},
	})
	out := runPurge(t)
	if strings.Contains(out, "kept_legacy=0") {
		t.Fatalf("dry-run did not keep the legacy key:\n%s", out)
	}
	if !bytes.Equal(waitFile(t, a, docLocation(doc), payload), payload) {
		t.Fatal("legacy document was deleted")
	}
}

func userPeer(t *testing.T, cfg Config, c *Client, name, first string) *mtproto.InputPeer {
	t.Helper()
	ph, err := cfg.Phone(name)
	if err != nil {
		t.Fatal(err)
	}
	cx, cancel := ctx()
	defer cancel()
	id, hash, err := c.ImportPhone(cx, ph.Number, first)
	if err != nil {
		t.Fatal(err)
	}
	return inputPeerUser(id, hash)
}

func sendVoiceBytes(t *testing.T, c *Client, to *mtproto.InputPeer, caption string, payload []byte) (*mtproto.Document, int32) {
	t.Helper()
	fileID := uploadBytes(t, c, payload)
	res := invokeOK(t, c, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Message:     caption,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File:     mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: "voice.ogg"}).To_InputFile(),
			MimeType: "audio/ogg",
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeAudio(&mtproto.DocumentAttribute{Voice: true, Duration_INT32: 1}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	for _, m := range allMessages(res) {
		if doc := m.GetMedia().GetDocument(); doc != nil && doc.GetId() != 0 {
			return doc, m.GetId()
		}
	}
	t.Fatalf("voice send returned no document (%T)", res)
	return nil, 0
}

func forwardReturning(t *testing.T, c *Client, from *mtproto.InputPeer, match func(*mtproto.Message) bool, to *mtproto.InputPeer) int32 {
	t.Helper()
	id := findBox(t, c, from, match)
	res := invokeOK(t, c, &mtproto.TLMessagesForwardMessages{
		Constructor: mtproto.CRC32_messages_forwardMessages_13704a7c,
		FromPeer:    from,
		Id:          []int32{id},
		RandomId:    []int64{randomID()},
		ToPeer:      to,
	})
	for _, m := range allMessages(res) {
		if match(m) {
			return m.GetId()
		}
	}
	t.Fatal("forward did not return the media")
	return 0
}

func findBox(t *testing.T, c *Client, peer *mtproto.InputPeer, match func(*mtproto.Message) bool) int32 {
	t.Helper()
	hist := invokeOK(t, c, &mtproto.TLMessagesGetHistory{
		Constructor: mtproto.CRC32_messages_getHistory,
		Peer:        peer,
		Limit:       50,
	})
	for _, m := range allMessages(hist) {
		if match(m) {
			return m.GetId()
		}
	}
	t.Fatal("message not in history")
	return 0
}

func messageColumn(t *testing.T, userID int64, boxID int32) string {
	t.Helper()
	return strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT message FROM messages WHERE user_id=%d AND user_message_box_id=%d`, userID, boxID)))
}

func blobRefCount(t *testing.T, documentID int64) string {
	t.Helper()
	return strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT COALESCE(MAX(b.ref_count),0) FROM media_blobs b JOIN media_blob_refs r ON r.blob_id=b.id WHERE r.ref_kind='document' AND r.ref_id=%d`, documentID)))
}

func waitPurgeQueue(t *testing.T, kind string, id int64) string {
	t.Helper()
	q := fmt.Sprintf(`SELECT COUNT(*) FROM media_purge_queue WHERE media_kind='%s' AND media_id=%d`, kind, id)
	deadline := time.Now().Add(15 * time.Second)
	var n string
	for {
		n = strings.TrimSpace(mustSQL(t, q))
		if n == "1" || time.Now().After(deadline) {
			return n
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func runPurge(t *testing.T) string {
	t.Helper()
	out, err := composeExec(2*time.Minute, "", "/app/bin/mediapurge", "-f", "/app/etc2/media.yaml", "-dry-run=false", "-backfill=false", "-grace=0")
	t.Logf("mediapurge:\n%s", out)
	if err != nil {
		t.Fatalf("mediapurge: %v\n%s", err, out)
	}
	return out
}
