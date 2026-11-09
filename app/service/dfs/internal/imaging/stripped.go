// Copyright 2022 Teamgram Authors
//  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//
// Author: teamgramio (teamgram.io@gmail.com)
//

package imaging

import (
	"bytes"
	"fmt"
	"image"
	"io"

	"github.com/teamgram/teamgram-server/app/service/dfs/internal/imaging/jpeg"
)

// StrippedJPEG builds the inline "i" thumb clients draw before upload.getFile
// returns. The input is a normal JPEG. The output is a Telegram stripped JPEG.
func StrippedJPEG(jpegBytes []byte) ([]byte, error) {
	if len(jpegBytes) == 0 {
		return nil, fmt.Errorf("empty jpeg")
	}
	img, err := Decode(bytes.NewReader(jpegBytes))
	if err != nil {
		return nil, err
	}
	if img == nil {
		return nil, fmt.Errorf("empty image")
	}
	b := img.Bounds()
	var small image.Image
	if b.Dx() >= b.Dy() {
		small = Resize(img, 40, 0)
	} else {
		small = Resize(img, 0, 40)
	}
	if small == nil || small.Bounds().Dx() <= 0 || small.Bounds().Dy() <= 0 {
		return nil, fmt.Errorf("empty stripped image")
	}
	var buf bytes.Buffer
	if err = EncodeStripped(&buf, small, 30); err != nil {
		return nil, err
	}
	if buf.Len() == 0 {
		return nil, fmt.Errorf("empty stripped jpeg")
	}
	return buf.Bytes(), nil
}

func EncodeStripped(w io.Writer, img image.Image, quality int) error {
	var (
		rgba *image.RGBA
		err  error
	)

	if nrgba, ok := img.(*image.NRGBA); ok {
		if nrgba.Opaque() {
			rgba = &image.RGBA{
				Pix:    nrgba.Pix,
				Stride: nrgba.Stride,
				Rect:   nrgba.Rect,
			}
		}
	}

	if rgba != nil {
		err = jpeg.EncodeStripped(w, rgba, &jpeg.Options{Quality: quality})
	} else {
		err = jpeg.EncodeStripped(w, img, &jpeg.Options{Quality: quality})
	}

	return err
}
