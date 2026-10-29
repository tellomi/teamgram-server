package core

import (
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
)

// seededDocumentFile serves a built-in sticker, emoji, or reaction icon from
// the process. Those bytes never go through dfs, so a missing R2 object cannot
// hold upload.getFile until the RPC deadline.
func seededDocumentFile(location *mtproto.InputFileLocation, offset int64, limit int32) (*mtproto.Upload_File, bool) {
	if location.GetPredicateName() != mtproto.Predicate_inputDocumentFileLocation {
		return nil, false
	}
	body, mime, ok := catalog.Must().DocumentFile(location.GetId())
	if !ok {
		return nil, false
	}
	// A .tgs has no jpeg thumb. Return an empty partial instead of asking dfs.
	if location.GetThumbSize() != "" && mime != "image/webp" {
		return uploadFileChunk(nil, "application/x-tgsticker"), true
	}
	return uploadFileChunk(sliceFile(body, offset, limit), mime), true
}

func sliceFile(body []byte, offset int64, limit int32) []byte {
	if offset < 0 {
		offset = 0
	}
	if offset >= int64(len(body)) {
		return []byte{}
	}
	rest := body[offset:]
	if limit > 0 && int(limit) < len(rest) {
		rest = rest[:limit]
	}
	out := make([]byte, len(rest))
	copy(out, rest)
	return out
}

func uploadFileChunk(body []byte, mime string) *mtproto.Upload_File {
	if body == nil {
		body = []byte{}
	}
	ft := mtproto.MakeTLStorageFilePartial(&mtproto.Storage_FileType{}).To_Storage_FileType()
	if mime == "image/webp" {
		ft = mtproto.MakeTLStorageFileWebp(&mtproto.Storage_FileType{}).To_Storage_FileType()
	}
	return mtproto.MakeTLUploadFile(&mtproto.Upload_File{
		Type:  ft,
		Mtime: int32(time.Now().Unix()),
		Bytes: body,
	}).To_Upload_File()
}
