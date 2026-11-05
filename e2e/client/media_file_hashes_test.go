package e2e

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// hashPart is the SHA-256 part size shared with upload.getCdnFileHashes
// (cdn.PartBytes). The e2e module does not import the server packages.
const hashPart = 128 * 1024

// TestMediaFileHashesAndBlobGC covers ADR-0083 B2.
// upload.getFileHashes returns 128 KiB SHA-256 parts, and offset skips
// earlier parts. Retention does not delete an object while another
// media_blob_refs row points at it. After the last reference is released,
// the object is deleted and media_blobs has no ref_count = 0 row. Deleting
// one message does not release the reference.
func TestMediaFileHashesAndBlobGC(t *testing.T) {
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

	unit := []byte("tellomi-b2-hashes\n")
	payload := bytes.Repeat(unit, (hashPart/len(unit))+8)
	if len(payload) <= hashPart {
		t.Fatalf("payload %d is not two parts", len(payload))
	}
	docA, _ := sendDocBytes(t, a, aToB, "application/octet-stream", "parts.bin", payload, false)
	docB, _ := sendDocBytes(t, b, bToA, "application/octet-stream", "parts.bin", payload, false)
	assertDocumentBlob(t, a, docA, docB, payload, "application/octet-stream")

	// Hash B only. Hashing A would cache "not expired" and hide the
	// FILE_ID_INVALID that retention sets on A.
	all := fileHashes(t, b, docLocation(docB), 0)
	if len(all) != 2 || all[0].GetOffset_INT64() != 0 || all[0].GetLimit() != hashPart {
		t.Fatalf("hashes %+v", all)
	}
	sum := sha256.Sum256(payload[:hashPart])
	tail := sha256.Sum256(payload[hashPart:])
	if !bytes.Equal(all[0].GetHash(), sum[:]) || !bytes.Equal(all[1].GetHash(), tail[:]) || all[1].GetLimit() != int32(len(payload)-hashPart) {
		t.Fatalf("hash bytes offset %d limit %d", all[1].GetOffset_INT64(), all[1].GetLimit())
	}
	rest := fileHashes(t, b, docLocation(docB), int64(hashPart))
	if len(rest) != 1 || !bytes.Equal(rest[0].GetHash(), all[1].GetHash()) {
		t.Fatalf("offset part %+v", rest)
	}
	mid := fileHashes(t, b, docLocation(docB), 1)
	if len(mid) != 1 || mid[0].GetOffset_INT64() != int64(hashPart) {
		t.Fatalf("offset 1 %+v", mid)
	}
	cx, cancel := ctx()
	_, err := a.Invoke(cx, &mtproto.TLUploadGetFileHashes{
		Constructor:  mtproto.CRC32_upload_getFileHashes_9156982a,
		Location:     docLocation(docB),
		Offset_INT64: -1,
	})
	cancel()
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "OFFSET_INVALID" {
		t.Fatalf("negative offset: %v", err)
	}

	// Deleting one message does not release the blob. This pair stays young
	// so the retention sweeps below do not expire it.
	small := bytes.Repeat([]byte("tellomi-b2-keep\n"), 20)
	keepA, msgA := sendDocBytes(t, a, aToB, "application/octet-stream", "keep.bin", small, false)
	keepB, _ := sendDocBytes(t, b, bToA, "application/octet-stream", "keep.bin", small, false)
	assertDocumentBlob(t, a, keepA, keepB, small, "application/octet-stream")
	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{msgA},
	})
	keepSum := sha256.Sum256(small)
	blob := strings.Fields(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*), COALESCE(MAX(ref_count), 0) FROM media_blobs WHERE sha256 = '%x' AND storage_class = 'documents'`, keepSum)))
	if len(blob) < 2 || blob[0] != "1" || blob[1] != "2" {
		t.Fatalf("delete message changed blob %q", blob)
	}
	if !bytes.Equal(waitFile(t, b, docLocation(keepB), small), small) {
		t.Fatal("other message broke after one message was deleted")
	}

	old := fmt.Sprint(time.Now().Add(-100 * 24 * time.Hour).Unix())
	mustSQL(t, fmt.Sprintf(`UPDATE documents SET date2 = %s WHERE document_id = %d`, old, docA.GetId()))
	first := sweep(t, false)
	if !strings.Contains(first, fmt.Sprintf("document %d (0 objects", docA.GetId())) {
		t.Fatalf("first expire removed the shared object:\n%s", first)
	}
	if !bytes.Equal(waitFile(t, b, docLocation(docB), payload), payload) {
		t.Fatal("shared object deleted while the other document is still live")
	}
	expectFileGone(t, a, docLocation(docA))
	sumHex := fmt.Sprintf("%x", sha256.Sum256(payload))
	blob = strings.Fields(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*), COALESCE(MAX(ref_count), 0) FROM media_blobs WHERE sha256 = '%s' AND storage_class = 'documents'`, sumHex)))
	if len(blob) < 2 || blob[0] != "1" || blob[1] != "1" {
		t.Fatalf("after first expire blob %q, want one row ref_count 1", blob)
	}
	if n := sqlScalar(t, `SELECT COUNT(*) FROM media_blobs WHERE ref_count = 0`); n != "0" {
		t.Fatalf("live ref_count = 0 rows: %s", n)
	}

	mustSQL(t, fmt.Sprintf(`UPDATE documents SET date2 = %s WHERE document_id = %d`, old, docB.GetId()))
	second := sweep(t, false)
	if !strings.Contains(second, fmt.Sprintf("document %d (1 objects", docB.GetId())) {
		t.Fatalf("last expire did not delete the object:\n%s", second)
	}
	if n := sqlScalar(t, fmt.Sprintf(`SELECT expired_at <> 0 FROM documents WHERE document_id = %d`, docB.GetId())); n != "1" {
		t.Fatalf("document %d expired_at flag %q", docB.GetId(), n)
	}
	blob = strings.Fields(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*), COALESCE(MAX(ref_count), 0) FROM media_blobs WHERE sha256 = '%s' AND storage_class = 'documents'`, sumHex)))
	if len(blob) < 1 || blob[0] != "0" {
		t.Fatalf("after last expire blob %q, want no row", blob)
	}
	if n := sqlScalar(t, `SELECT COUNT(*) FROM media_blobs WHERE ref_count = 0`); n != "0" {
		t.Fatalf("ref_count = 0 rows after last release: %s", n)
	}
	path := sqlScalar(t, fmt.Sprintf(`SELECT file_path FROM documents WHERE document_id = %d`, docA.GetId()))
	if path == "" {
		t.Fatal("origin path missing")
	}
	if n := sqlScalar(t, fmt.Sprintf(
		`SELECT COUNT(*) FROM media_blob_gc WHERE storage_class = 'documents' AND file_path = '%s'`, path)); n != "0" {
		t.Fatalf("gc row left for %s: %s", path, n)
	}
}

func fileHashes(t *testing.T, c *Client, loc *mtproto.InputFileLocation, offset int64) []*mtproto.FileHash {
	t.Helper()
	res := invokeOK(t, c, &mtproto.TLUploadGetFileHashes{
		Constructor:  mtproto.CRC32_upload_getFileHashes_9156982a,
		Location:     loc,
		Offset_INT64: offset,
	})
	vec, ok := res.(*tlVector)
	if !ok || len(vec.Hashes) == 0 || int(vec.Count) != len(vec.Hashes) {
		t.Fatalf("getFileHashes returned %T", res)
	}
	return vec.Hashes
}

func expectFileGone(t *testing.T, c *Client, loc *mtproto.InputFileLocation) {
	t.Helper()
	cx, cancel := ctx()
	defer cancel()
	_, err := c.Invoke(cx, &mtproto.TLUploadGetFile{
		Constructor: mtproto.CRC32_upload_getFile_be5335be,
		Location:    loc,
		Limit:       64 * 1024,
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Message != "FILE_ID_INVALID" {
		t.Fatalf("getFile err %v, want FILE_ID_INVALID", err)
	}
}

func sqlScalar(t *testing.T, q string) string {
	t.Helper()
	lines := nonEmptyLines(mustSQL(t, q))
	if len(lines) == 0 {
		return ""
	}
	return strings.TrimSpace(lines[0])
}
