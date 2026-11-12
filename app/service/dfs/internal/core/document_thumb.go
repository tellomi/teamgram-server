package core

import (
	"bytes"
	"image"

	"github.com/teamgram/marmota/pkg/bytes2"
	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging"
)

// resizeDocumentThumbs builds the stripped and medium previews from an
// already oriented image. Callers must decode with EXIF orientation applied
// first, so these sizes match documentAttributeImageSize.
func resizeDocumentThumbs(thumb image.Image) (stripped []byte, medium image.Image, err error) {
	buf := bytes2.NewBuffer(make([]byte, 0, 4096))
	if thumb.Bounds().Dx() >= thumb.Bounds().Dy() {
		err = imaging.EncodeStripped(buf, imaging.Resize(thumb, 40, 0), 30)
		medium = imaging.Resize(thumb, 320, 0)
	} else {
		err = imaging.EncodeStripped(buf, imaging.Resize(thumb, 0, 40), 30)
		medium = imaging.Resize(thumb, 0, 320)
	}
	if err != nil {
		return nil, nil, err
	}
	return buf.Bytes(), medium, nil
}

// documentFileThumbs decodes a file JPEG with EXIF orientation and builds
// the thumbs DfsUploadDocumentFileV2 stores. decErr means the bytes are not
// an image and the upload continues without thumbs. buildErr means the
// preview could not be encoded. The input bytes are not modified.
func documentFileThumbs(data []byte) (stripped []byte, medium image.Image, decErr, buildErr error) {
	thumb, err := imaging.DecodeOriented(bytes.NewReader(data))
	if err != nil {
		return nil, nil, err, nil
	}
	stripped, medium, err = resizeDocumentThumbs(thumb)
	if err != nil {
		return nil, nil, nil, err
	}
	return stripped, medium, nil, nil
}
