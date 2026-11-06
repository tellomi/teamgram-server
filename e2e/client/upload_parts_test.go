package e2e

import (
	"bytes"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

// TestUploadPartChecks is the client-visible side of the chunked upload
// rules. A non-last big part that does not divide 512 KiB is rejected at
// save time. Finalize of a photo or document whose declared part is absent
// returns FILE_PART_<n>_MISSING instead of publishing a short file.
func TestUploadPartChecks(t *testing.T) {
	cfg := requireE2E(t)
	c, _ := loginNamed(t, cfg, ProfileIOS, "parts", "Parts")
	cx, cancel := ctx()
	defer cancel()

	_, err := c.Invoke(cx, &mtproto.TLUploadSaveBigFilePart{
		Constructor:    mtproto.CRC32_upload_saveBigFilePart,
		FileId:         randomID(),
		FilePart:       0,
		FileTotalParts: 2,
		Bytes:          bytes.Repeat([]byte{1}, 100*1024),
	})
	rpc, ok := asRPCError(err)
	if !ok || rpc.Code != 400 || rpc.Message != "FILE_PART_SIZE_INVALID" {
		t.Fatalf("illegal part size: %v", err)
	}

	fileID := randomID()
	_, err = c.Invoke(cx, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      fileID,
		FilePart:    0,
		Bytes:       bytes.Repeat([]byte{2}, 16*1024),
	})
	if err != nil {
		t.Fatalf("save part 0: %v", err)
	}

	_, err = c.Invoke(cx, &mtproto.TLMessagesUploadMedia{
		Constructor: mtproto.CRC32_messages_uploadMedia_14967978,
		Peer:        inputPeerSelf(),
		Media: mtproto.MakeTLInputMediaUploadedPhoto(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: fileID,
				Parts:    2,
				Name:     "missing.jpg",
			}).To_InputFile(),
		}).To_InputMedia(),
	})
	rpc, ok = asRPCError(err)
	if !ok || rpc.Code != 400 || rpc.Message != "FILE_PART_1_MISSING" {
		t.Fatalf("photo missing part: %v", err)
	}

	docID := randomID()
	_, err = c.Invoke(cx, &mtproto.TLUploadSaveFilePart{
		Constructor: mtproto.CRC32_upload_saveFilePart,
		FileId:      docID,
		FilePart:    0,
		Bytes:       bytes.Repeat([]byte{3}, 16*1024),
	})
	if err != nil {
		t.Fatalf("save document part 0: %v", err)
	}
	_, err = c.Invoke(cx, &mtproto.TLMessagesUploadMedia{
		Constructor: mtproto.CRC32_messages_uploadMedia_14967978,
		Peer:        inputPeerSelf(),
		Media: mtproto.MakeTLInputMediaUploadedDocument(&mtproto.InputMedia{
			File: mtproto.MakeTLInputFile(&mtproto.InputFile{
				Id_INT64: docID,
				Parts:    2,
				Name:     "missing.bin",
			}).To_InputFile(),
			MimeType: "application/octet-stream",
			Attributes: []*mtproto.DocumentAttribute{
				mtproto.MakeTLDocumentAttributeFilename(&mtproto.DocumentAttribute{
					FileName: "missing.bin",
				}).To_DocumentAttribute(),
			},
		}).To_InputMedia(),
	})
	rpc, ok = asRPCError(err)
	if !ok || rpc.Code != 400 || rpc.Message != "FILE_PART_1_MISSING" {
		t.Fatalf("document missing part: %v", err)
	}
}
