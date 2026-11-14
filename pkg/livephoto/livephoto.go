// Package livephoto carries the video half of a Telegram live photo
// (layer 232) from the InputMedia a client sends to the MessageMedia the
// server stores and delivers:
//
//	inputMediaUploadedPhoto#7d8375da ... live_photo:flags.3?true ... video:flags.3?InputDocument
//	inputMediaPhoto#e3af4434 ... live_photo:flags.2?true ... video:flags.2?InputDocument
//	messageMediaPhoto#e216eb63 ... live_photo:flags.4?true ... video:flags.4?Document
//
// live_photo and video share one flag bit on the wire, so a MessageMedia
// must carry both or neither. Encoding live_photo without a video sets the
// bit but writes no Document, and clients fail to parse the message.
package livephoto

import (
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// Resolver turns the client's InputDocument into the stored Document. It
// must check access_hash and file_reference the same way inputMediaDocument
// does, and return MEDIA_INVALID when they do not match.
type Resolver func(in *mtproto.InputDocument) (*mtproto.Document, error)

// Video resolves the live photo video on in. A plain photo returns nil, nil.
// A video that cannot be resolved, or is not a video, is MEDIA_INVALID: the
// send fails instead of silently dropping to a still photo.
func Video(in *mtproto.InputMedia, resolve Resolver) (*mtproto.Document, error) {
	if in == nil {
		return nil, nil
	}
	v := in.GetVideo()
	if v == nil || v.GetPredicateName() == mtproto.Predicate_inputDocumentEmpty {
		return nil, nil
	}
	if resolve == nil || v.GetId() == 0 {
		return nil, mtproto.ErrMediaInvalid
	}
	doc, err := resolve(v)
	if err != nil {
		return nil, err
	}
	if !IsVideo(doc) {
		return nil, mtproto.ErrMediaInvalid
	}
	return doc, nil
}

// IsVideo reports whether doc can be a live photo video: a video/* mime type
// or a documentAttributeVideo.
func IsVideo(doc *mtproto.Document) bool {
	if doc == nil || doc.GetId() == 0 || doc.GetPredicateName() == mtproto.Predicate_documentEmpty {
		return false
	}
	if strings.HasPrefix(strings.ToLower(doc.GetMimeType()), "video/") {
		return true
	}
	for _, a := range doc.GetAttributes() {
		if a.GetPredicateName() == mtproto.Predicate_documentAttributeVideo {
			return true
		}
	}
	return false
}

// Set puts video on a messageMediaPhoto and keeps live_photo in step with it.
func Set(mm *mtproto.MessageMedia, video *mtproto.Document) *mtproto.MessageMedia {
	if mm == nil {
		return nil
	}
	mm.Video_FLAGDOCUMENT = video
	mm.LivePhoto = video != nil
	return mm
}
