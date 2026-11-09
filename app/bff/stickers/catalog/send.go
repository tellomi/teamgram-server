package catalog

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
	"github.com/teamgram/teamgram-server/pkg/mediaaccess"
)

// CatalogDocument resolves a built-in sticker or emoji. found is false when
// the id is not in the catalog, so the caller can look in the media table.
// A wrong access hash, or an emoji on the blocklist, is MEDIA_INVALID.
func (s *Service) CatalogDocument(id, accessHash int64) (doc *mtproto.Document, found bool, err error) {
	if s == nil || id == 0 {
		return nil, false, nil
	}
	s.mu.Lock()
	link := s.docs[id]
	s.mu.Unlock()
	if link == nil || link.item == nil {
		return nil, false, nil
	}
	if emojiblock.Blocked(link.item.Emoticon) {
		return nil, true, mtproto.ErrMediaInvalid
	}
	if !mediaaccess.SameHash(accessHash, link.item.AccessHash) {
		if mediaaccess.MismatchAllowed("sticker", id) {
			return link.item.document(link.pack), true, nil
		}
		return nil, true, mtproto.ErrMediaInvalid
	}
	return link.item.document(link.pack), true, nil
}

// DocumentAccess is the access_hash issued for a catalog sticker, emoji, or
// reaction. public is false for a pack a user created. ok is false when the
// id is not in the catalog.
func (s *Service) DocumentAccess(id int64) (hash int64, public, ok bool) {
	if s == nil || id == 0 {
		return 0, false, false
	}
	s.mu.Lock()
	link := s.docs[id]
	s.mu.Unlock()
	if link == nil || link.item == nil {
		return 0, false, false
	}
	return link.item.AccessHash, link.pack == nil || link.pack.Owner == 0, true
}

// PublicDocumentID reports a sticker, emoji, or reaction from an official pack.
func (s *Service) PublicDocumentID(id int64) bool {
	_, public, ok := s.DocumentAccess(id)
	return ok && public
}

// ResolveInputDocument is the document for inputMediaDocument.
// The sticker catalog is checked first. The media table is consulted only
// on a miss. An empty document is never returned.
func ResolveInputDocument(in *mtproto.InputDocument, fetch func() (*mtproto.Document, error)) (*mtproto.Document, error) {
	if in == nil || in.GetPredicateName() == mtproto.Predicate_inputDocumentEmpty || in.GetId() == 0 {
		return nil, mtproto.ErrMediaInvalid
	}
	doc, found, err := Must().CatalogDocument(in.GetId(), in.GetAccessHash())
	if err != nil {
		return nil, err
	}
	if found {
		return doc, nil
	}
	if fetch == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	stored, storedErr := fetch()
	doc, err = UsableStoredDocument(stored, storedErr)
	if err != nil {
		return nil, err
	}
	if !mediaaccess.SameHash(in.GetAccessHash(), doc.GetAccessHash()) {
		if !mediaaccess.MismatchAllowed("inputDocument", in.GetId()) {
			return nil, mtproto.ErrMediaInvalid
		}
	} else if err = mediaaccess.Reference(in.GetFileReference(), doc.GetFileReference()); err != nil {
		if !mediaaccess.MismatchAllowed("inputDocument", in.GetId()) {
			return nil, err
		}
	}
	return doc, nil
}

// ResolveInputPhoto is the photo for inputMediaPhoto. The access_hash must
// be the one stored on the photo. The returned photo is that row, so the
// outgoing message carries the issued hash rather than the client's copy.
func ResolveInputPhoto(in *mtproto.InputPhoto, fetch func() (*mtproto.Photo, error)) (*mtproto.Photo, error) {
	if in == nil || in.GetPredicateName() == mtproto.Predicate_inputPhotoEmpty || in.GetId() == 0 {
		return nil, mtproto.ErrMediaInvalid
	}
	if fetch == nil {
		return nil, mtproto.ErrMediaInvalid
	}
	photo, err := fetch()
	if err != nil || photo == nil || photo.GetPredicateName() != mtproto.Predicate_photo || photo.GetId() == 0 {
		return nil, mtproto.ErrMediaInvalid
	}
	if !mediaaccess.SameHash(in.GetAccessHash(), photo.GetAccessHash()) {
		if !mediaaccess.MismatchAllowed("inputPhoto", in.GetId()) {
			return nil, mtproto.ErrMediaInvalid
		}
	} else if err = mediaaccess.Reference(in.GetFileReference(), photo.GetFileReference()); err != nil {
		if !mediaaccess.MismatchAllowed("inputPhoto", in.GetId()) {
			return nil, err
		}
	}
	return photo, nil
}

// UsableStoredDocument accepts a media-table document. documentEmpty, a
// lookup error, and a missing mime type are MEDIA_INVALID. Callers must not
// attach an empty document to a message.
func UsableStoredDocument(stored *mtproto.Document, storedErr error) (*mtproto.Document, error) {
	if storedErr != nil || stored == nil || stored.GetPredicateName() != mtproto.Predicate_document || stored.GetId() == 0 || stored.GetMimeType() == "" {
		return nil, mtproto.ErrMediaInvalid
	}
	return stored, nil
}
