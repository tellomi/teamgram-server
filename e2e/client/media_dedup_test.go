package e2e

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
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

// TestMediaDedupPhotoGifAndFile uploads one JPEG as a photo, the same JPEG
// as an image document, one plain file, and one small GIF from two accounts.
// Each pair shares one blob and keeps two rows. Deleting one photo message
// leaves the other photo downloadable. A different JPEG does not reuse the
// first photo's object.
func TestMediaDedupPhotoGifAndFile(t *testing.T) {
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

	jpegBytes := testJPEG(t, 480, 360, 1)
	jpegSum := sha256.Sum256(jpegBytes)
	photoA, msgA := sendPhotoBytes(t, a, aToB, "same.jpg", jpegBytes)
	photoB, _ := sendPhotoBytes(t, b, bToA, "same.jpg", jpegBytes)
	if photoA.GetId() == 0 || photoB.GetId() == 0 || photoA.GetId() == photoB.GetId() {
		t.Fatalf("photo ids %d and %d", photoA.GetId(), photoB.GetId())
	}
	if photoA.GetAccessHash() == photoB.GetAccessHash() {
		t.Fatal("reused photo kept the same access_hash")
	}
	assertPhotoBlob(t, photoA.GetId(), photoB.GetId(), hex.EncodeToString(jpegSum[:]))
	size := largestPhotoSize(t, photoA)
	gotA := waitNonEmpty(t, a, photoLocation(photoA, size))
	gotB := waitNonEmpty(t, b, photoLocation(photoB, size))
	if !bytes.Equal(gotA, gotB) {
		t.Fatalf("photo %s downloads differ (%d vs %d)", size, len(gotA), len(gotB))
	}
	invokeOK(t, a, &mtproto.TLMessagesDeleteMessages{
		Constructor: mtproto.CRC32_messages_deleteMessages,
		Revoke:      true,
		Id:          []int32{msgA},
	})
	if !bytes.Equal(waitNonEmpty(t, b, photoLocation(photoB, size)), gotB) {
		t.Fatal("bob's photo broke after alice deleted her message")
	}
	if !bytes.Equal(waitNonEmpty(t, a, photoLocation(photoA, size)), gotA) {
		t.Fatal("alice's photo broke after deleting the message")
	}

	other := testJPEG(t, 480, 360, 9)
	photoC, _ := sendPhotoBytes(t, a, aToB, "other.jpg", other)
	pathC := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT file_path FROM media_photo_paths WHERE photo_id = %d AND size_type = '%s'`, photoC.GetId(), size)))
	shared := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT file_path FROM media_photo_paths WHERE photo_id = %d AND size_type = '%s'`, photoA.GetId(), size)))
	if pathC == "" || pathC == shared {
		t.Fatalf("different jpeg reused %q (shared was %s)", pathC, shared)
	}

	docA, _ := sendDocBytes(t, a, aToB, "image/jpeg", "same.jpg", jpegBytes, false)
	docB, _ := sendDocBytes(t, b, bToA, "image/jpeg", "same.jpg", jpegBytes, false)
	assertDocumentBlob(t, a, docA, docB, jpegBytes, "image/jpeg")
	if !bytes.Equal(waitFile(t, a, docLocation(docA), jpegBytes), jpegBytes) ||
		!bytes.Equal(waitFile(t, b, docLocation(docB), jpegBytes), jpegBytes) {
		t.Fatal("image document downloads differ from the jpeg")
	}
	assertThumbShared(t, docA.GetId(), docB.GetId())

	plain := bytes.Repeat([]byte("tellomi-dedup-file-v1\n"), 200)
	fileA, _ := sendDocBytes(t, a, aToB, "application/octet-stream", "notes.bin", plain, false)
	fileB, _ := sendDocBytes(t, b, bToA, "application/octet-stream", "notes.bin", plain, false)
	assertDocumentBlob(t, a, fileA, fileB, plain, "application/octet-stream")
	if !bytes.Equal(waitFile(t, a, docLocation(fileA), plain), plain) ||
		!bytes.Equal(waitFile(t, b, docLocation(fileB), plain), plain) {
		t.Fatal("plain document downloads differ")
	}

	gif := tinyGIF()
	gifA, _ := sendDocBytes(t, a, aToB, "image/gif", "anim.gif", gif, true)
	gifB, _ := sendDocBytes(t, b, bToA, "image/gif", "anim.gif", gif, true)
	assertDocumentBlob(t, a, gifA, gifB, gif, "image/gif")
	if !bytes.Equal(waitFile(t, a, docLocation(gifA), gif), gif) ||
		!bytes.Equal(waitFile(t, b, docLocation(gifB), gif), gif) {
		t.Fatal("gif downloads differ from the upload")
	}
	assertThumbShared(t, gifA.GetId(), gifB.GetId())
}

func testJPEG(t *testing.T, w, h int, seed byte) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x) + seed, uint8(y), seed, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func tinyGIF() []byte {
	return []byte{
		0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00, 0x00,
		0xff, 0xff, 0xff, 0x00, 0x00, 0x00, 0x21, 0xf9, 0x04, 0x01, 0x00, 0x00, 0x00,
		0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0x02, 0x02,
		0x44, 0x01, 0x00, 0x3b,
	}
}

func sendPhotoBytes(t *testing.T, c *Client, to *mtproto.InputPeer, name string, payload []byte) (*mtproto.Photo, int32) {
	t.Helper()
	fileID := uploadBytes(t, c, payload)
	res := invokeOK(t, c, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: name}).To_InputFile(),
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	for _, m := range allMessages(res) {
		if p := m.GetMedia().GetPhoto_FLAGPHOTO(); p != nil && p.GetId() != 0 {
			return p, m.GetId()
		}
	}
	t.Fatalf("sendMedia photo returned no photo (%T)", res)
	return nil, 0
}

func sendDocBytes(t *testing.T, c *Client, to *mtproto.InputPeer, mime, name string, payload []byte, animated bool) (*mtproto.Document, int32) {
	t.Helper()
	fileID := uploadBytes(t, c, payload)
	attrs := []*mtproto.DocumentAttribute{
		mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{FileName: name}).To_DocumentAttribute(),
	}
	if animated {
		attrs = append(attrs, mtproto.MakeTLDocumentAttributeAnimated(&mtproto.DocumentAttribute{}).To_DocumentAttribute())
	}
	res := invokeOK(t, c, &mtproto.TLMessagesSendMedia{
		Constructor: mtproto.CRC32_messages_sendMedia_7852834e,
		Peer:        to,
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File:       mtproto.MakeTLInputFile(&mtproto.InputFile{Id_INT64: fileID, Parts: 1, Name: name}).To_InputFile(),
			MimeType:   mime,
			Attributes: attrs,
		}).To_InputMedia(),
		RandomId: randomID(),
	})
	for _, m := range allMessages(res) {
		if doc := m.GetMedia().GetDocument(); doc != nil && doc.GetId() != 0 {
			return doc, m.GetId()
		}
	}
	t.Fatalf("sendMedia %s returned no document (%T)", mime, res)
	return nil, 0
}

func assertPhotoBlob(t *testing.T, idA, idB int64, sumHex string) {
	t.Helper()
	rows := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT photo_id, access_hash, sha256, blob_id FROM photos WHERE photo_id IN (%d, %d) ORDER BY photo_id`, idA, idB)))
	lines := nonEmptyLines(rows)
	if len(lines) != 2 {
		t.Fatalf("photos rows:\n%s", rows)
	}
	colsA := strings.Split(lines[0], "\t")
	colsB := strings.Split(lines[1], "\t")
	if len(colsA) < 4 || len(colsB) < 4 {
		t.Fatalf("photo row shape:\n%s", rows)
	}
	if colsA[2] != sumHex || colsB[2] != sumHex {
		t.Fatalf("photo sha256 %s and %s, want %s", colsA[2], colsB[2], sumHex)
	}
	if colsA[3] == "0" || colsA[3] != colsB[3] {
		t.Fatalf("photo blob_id not shared:\n%s", rows)
	}
	blob := strings.Fields(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*), COALESCE(MAX(ref_count), 0) FROM media_blobs WHERE sha256 = '%s' AND storage_class = 'photos'`, sumHex)))
	if len(blob) < 2 || blob[0] != "1" || blob[1] != "2" {
		t.Fatalf("photo media_blobs count/ref_count %q, want one row with ref_count 2", blob)
	}
	assertSizePathsShared(t, idA, idB)
}

func assertDocumentBlob(t *testing.T, c *Client, docA, docB *mtproto.Document, payload []byte, mime string) {
	t.Helper()
	if docA.GetId() == 0 || docB.GetId() == 0 || docA.GetId() == docB.GetId() {
		t.Fatalf("%s document ids %d and %d", mime, docA.GetId(), docB.GetId())
	}
	if docA.GetAccessHash() == docB.GetAccessHash() {
		t.Fatalf("%s reused upload kept the same access_hash", mime)
	}
	sum := sha256.Sum256(payload)
	sumHex := hex.EncodeToString(sum[:])
	rows := strings.TrimSpace(mustSQL(t, fmt.Sprintf(
		`SELECT document_id, access_hash, file_path, sha256, blob_id FROM documents WHERE document_id IN (%d, %d) ORDER BY document_id`,
		docA.GetId(), docB.GetId())))
	lines := nonEmptyLines(rows)
	if len(lines) != 2 {
		t.Fatalf("%s documents rows:\n%s", mime, rows)
	}
	colsA := strings.Split(lines[0], "\t")
	colsB := strings.Split(lines[1], "\t")
	if len(colsA) < 5 || len(colsB) < 5 {
		t.Fatalf("%s row shape:\n%s", mime, rows)
	}
	if colsA[2] == "" || colsA[2] != colsB[2] {
		t.Fatalf("%s file_path not shared:\n%s", mime, rows)
	}
	if colsB[2] == fmt.Sprintf("%d.dat", docB.GetId()) {
		t.Fatalf("%s second document wrote its own object key:\n%s", mime, rows)
	}
	if colsA[3] != sumHex || colsB[3] != sumHex {
		t.Fatalf("%s sha256 %s and %s, want %s", mime, colsA[3], colsB[3], sumHex)
	}
	if colsA[4] == "0" || colsA[4] != colsB[4] {
		t.Fatalf("%s blob_id not shared:\n%s", mime, rows)
	}
	blob := strings.Fields(mustSQL(t, fmt.Sprintf(
		`SELECT COUNT(*), COALESCE(MAX(ref_count), 0) FROM media_blobs WHERE sha256 = '%s' AND storage_class = 'documents'`, sumHex)))
	if len(blob) < 2 || blob[0] != "1" || blob[1] != "2" {
		t.Fatalf("%s media_blobs count/ref_count %q, want one row with ref_count 2", mime, blob)
	}
	byHash := documentFromRPC(invokeOK(t, c, &mtproto.TLMessagesGetDocumentByHash{
		Constructor: mtproto.CRC32_messages_getDocumentByHash_b1f2061f,
		Sha256:      sum[:],
		Size2_INT64: int64(len(payload)),
		MimeType:    mime,
	}))
	if byHash.GetPredicateName() == mtproto.Predicate_documentEmpty || (byHash.GetId() != docA.GetId() && byHash.GetId() != docB.GetId()) {
		t.Fatalf("%s getDocumentByHash returned %s id %d", mime, byHash.GetPredicateName(), byHash.GetId())
	}
}

func objectOwnedBy(path string, id int64) bool {
	suffix := fmt.Sprintf("%d.dat", id)
	return path == suffix || strings.HasSuffix(path, "/"+suffix)
}

func assertSizePathsShared(t *testing.T, idA, idB int64) {
	t.Helper()
	rows := mustSQL(t, fmt.Sprintf(
		`SELECT photo_id, size_type, file_path FROM media_photo_paths WHERE photo_id IN (%d, %d) AND size_type <> 'i' ORDER BY size_type, photo_id`,
		idA, idB))
	got := map[string]map[string]string{}
	for _, line := range nonEmptyLines(rows) {
		cols := strings.Split(line, "\t")
		if len(cols) < 3 || cols[2] == "" {
			t.Fatalf("photo path row %q", line)
		}
		if got[cols[1]] == nil {
			got[cols[1]] = map[string]string{}
		}
		got[cols[1]][cols[0]] = cols[2]
	}
	if len(got) == 0 {
		t.Fatalf("no media_photo_paths for %d and %d", idA, idB)
	}
	aID := fmt.Sprintf("%d", idA)
	bID := fmt.Sprintf("%d", idB)
	for size, paths := range got {
		if paths[aID] == "" || paths[aID] != paths[bID] {
			t.Fatalf("size %s paths not shared: %#v", size, paths)
		}
		if !objectOwnedBy(paths[aID], idA) {
			t.Fatalf("size %s path %s is not the first upload's object", size, paths[aID])
		}
		if objectOwnedBy(paths[bID], idB) {
			t.Fatalf("size %s second upload wrote %s", size, paths[bID])
		}
	}
}

func assertThumbShared(t *testing.T, idA, idB int64) {
	t.Helper()
	rows := mustSQL(t, fmt.Sprintf(
		`SELECT photo_id, size_type, file_path FROM media_photo_paths WHERE photo_id IN (%d, %d) AND size_type IN ('s','m','x','y','w') ORDER BY size_type, photo_id`,
		idA, idB))
	lines := nonEmptyLines(rows)
	if len(lines) < 2 {
		t.Fatalf("document thumbs not recorded:\n%s", rows)
	}
	assertSizePathsShared(t, idA, idB)
}

func largestPhotoSize(t *testing.T, p *mtproto.Photo) string {
	t.Helper()
	var best string
	var n int32
	for _, sz := range p.GetSizes() {
		if sz.GetPredicateName() != mtproto.Predicate_photoSize || sz.GetType() == "" {
			continue
		}
		if sz.GetSize2() >= n {
			n = sz.GetSize2()
			best = sz.GetType()
		}
	}
	if best == "" {
		t.Fatalf("photo %d has no file size", p.GetId())
	}
	return best
}

func waitNonEmpty(t *testing.T, c *Client, loc *mtproto.InputFileLocation) []byte {
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
		if err == nil && len(got) > 0 {
			return got
		}
		last = err
		if time.Now().After(deadline) {
			t.Fatalf("getFile: err %v bytes %d", last, len(got))
		}
		time.Sleep(300 * time.Millisecond)
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
