package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestMediaDedupTwoAccounts uploads one video from two accounts. Object
// storage keeps a single key; each account gets its own document id and
// access_hash. Deleting one message leaves the other download working.
// messages.getDocumentByHash returns one of those documents, and a different
// mime returns documentEmpty.
func TestMediaDedupTwoAccounts(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "dedup_a", "DedupA")
	b, _ := loginNamed(t, cfg, ProfileAndroid, "dedup_b", "DedupB")

	peer := func(c *Client, name, first string) *mtproto.InputPeer {
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
	aToB := peer(a, "dedup_b", "DedupB")
	bToA := peer(b, "dedup_a", "DedupA")

	payload := bytes.Repeat([]byte("tellomi-dedup-video-v1\n"), 300)
	sum := sha256.Sum256(payload)
	sumHex := hex.EncodeToString(sum[:])

	docA, msgA := sendPayload(t, a, aToB, "video/mp4", "clip.mp4", payload)
	docB, _ := sendPayload(t, b, bToA, "video/mp4", "clip.mp4", payload)
	if docA.GetId() == 0 || docB.GetId() == 0 || docA.GetId() == docB.GetId() {
		t.Fatalf("document ids %d and %d", docA.GetId(), docB.GetId())
	}
	if docA.GetAccessHash() == docB.GetAccessHash() {
		t.Fatal("reused upload kept the same access_hash")
	}

	rows := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT document_id, access_hash, file_path, sha256, blob_id, file_size FROM documents WHERE document_id IN (%d, %d) ORDER BY document_id`,
		docA.GetId(), docB.GetId())))
	lines := nonEmptyLines(rows)
	if len(lines) != 2 {
		t.Fatalf("documents rows:\n%s", rows)
	}
	colsA := strings.Split(lines[0], "\t")
	colsB := strings.Split(lines[1], "\t")
	if len(colsA) < 6 || len(colsB) < 6 {
		t.Fatalf("row shape:\n%s", rows)
	}
	if colsA[2] == "" || colsA[2] != colsB[2] {
		t.Fatalf("file_path not shared:\n%s", rows)
	}
	if colsB[2] == fmt.Sprintf("%d.dat", docB.GetId()) {
		t.Fatalf("second document wrote its own object key:\n%s", rows)
	}
	if colsA[3] != sumHex || colsB[3] != sumHex {
		t.Fatalf("sha256 %s and %s, want %s", colsA[3], colsB[3], sumHex)
	}
	if colsA[4] == "0" || colsA[4] != colsB[4] {
		t.Fatalf("blob_id not shared:\n%s", rows)
	}
	blob := strings.Fields(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*), COALESCE(MAX(ref_count), 0) FROM media_blobs WHERE sha256 = '%s' AND storage_class = 'documents'`, sumHex)))
	if len(blob) < 2 || blob[0] != "1" || blob[1] != "2" {
		t.Fatalf("media_blobs count/ref_count %q, want one row with ref_count 2", blob)
	}

	gotA := waitFile(t, a, docLocation(docA), payload)
	gotB := waitFile(t, b, docLocation(docB), payload)
	if !bytes.Equal(gotA, payload) || !bytes.Equal(gotB, payload) {
		t.Fatalf("downloads differ from the upload (%d, %d bytes)", len(gotA), len(gotB))
	}

	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{msgA},
	})
	// Alice's message is gone. Bob's own upload still downloads, and so does
	// the shared object behind Alice's document id.
	if !bytes.Equal(waitFile(t, b, docLocation(docB), payload), payload) {
		t.Fatal("bob's download broke after alice deleted her message")
	}
	if !bytes.Equal(waitFile(t, a, docLocation(docA), payload), payload) {
		t.Fatal("alice's document broke after deleting the message")
	}

	byHash := documentFromRPC(invokeOK(t, a, &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Sha256:      sum[:],
		Size2_INT64: int64(len(payload)),
		MimeType:    "video/mp4",
	}))
	if byHash.GetId() != docA.GetId() && byHash.GetId() != docB.GetId() {
		t.Fatalf("getDocumentByHash id %d", byHash.GetId())
	}
	if byHash.GetPredicateName() == mtproto.Predicate_documentEmpty {
		t.Fatal("getDocumentByHash returned documentEmpty")
	}
	miss := documentFromRPC(invokeOK(t, a, &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Sha256:      sum[:],
		Size2_INT64: int64(len(payload)),
		MimeType:    "application/octet-stream",
	}))
	if miss.GetPredicateName() != mtproto.Predicate_documentEmpty {
		t.Fatalf("wrong mime returned %s id %d", miss.GetPredicateName(), miss.GetId())
	}

	other := bytes.Repeat([]byte("tellomi-dedup-video-v2\n"), 300)
	docC, _ := sendPayload(t, a, aToB, "video/mp4", "other.mp4", other)
	pathC := strings.TrimSpace(mustSQL(t, fmt.Sprintf(`SELECT file_path FROM documents WHERE document_id = %d`, docC.GetId())))
	if pathC == "" || pathC == colsA[2] {
		t.Fatalf("different bytes reused %q (shared was %s)", pathC, colsA[2])
	}
}

func sendPayload(t *testing.T, c *Client, to *mtproto.InputPeer, mime, name string, payload []byte) (*mtproto.Document, int32) {
	t.Helper()
	fileID := uploadBytes(t, c, payload)
	res := invokeOK(t, c, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File:     mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: name}).To_InputFile(),
			MimeType: mime,
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: name}).To_DocumentAttribute(),
				mtproto.MakeTLDocumentAttributeVideo(&mtproto.DocumentAttribute{
					Duration: 1,
					W:        160,
					H:        120,
				}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	for _, m := range allMessages(res) {
		if doc := m.GetMedia().GetDocument(); doc != nil && doc.GetId() != 0 {
			return doc, m.GetId()
		}
	}
	t.Fatalf("sendMedia returned no document (%T)", res)
	return nil, 0
}

func waitFile(t *testing.T, c *Client, loc *mtproto.InputFileLocation, want []byte) []byte {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	var last error
	for {
		cx, cancel := ctx()
		res, err := c.Invoke(cx, &mtproto.TLUploadGetFile{
			Constructor: mtproto.CRC32_upload_getFile_be5335be,
			Location:    loc,
			Limit:       512 * 1024,
		})
		cancel()
		got := bytesOf(res)
		if err == nil && bytes.Equal(got, want) {
			return got
		}
		last = err
		if time.Now().After(deadline) {
			t.Fatalf("getFile: err %v bytes %d", last, len(got))
		}
		time.Sleep(300 * time.Millisecond)
	}
}

func documentFromRPC(obj mtproto.TLObject) *mtproto.Document {
	switch d := obj.(type) {
	case *mtproto.TLDocument:
		return d.To_Document()
	case *mtproto.TLDocumentEmpty:
		return d.To_Document()
	case *mtproto.Document:
		return d
	default:
		return mtproto.MakeTLDocumentEmpty(nil).To_Document()
	}
}

func nonEmptyLines(s string) []string {
	var out []string
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if l != "" && !strings.HasPrefix(l, "mysql:") {
			out = append(out, l)
		}
	}
	return out
}
