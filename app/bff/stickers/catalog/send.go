package catalog

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/emojiblock"
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
	if accessHash != 0 && accessHash != link.item.AccessHash {
		return nil, true, mtproto.ErrMediaInvalid
	}
	return link.item.document(link.pack), true, nil
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
	if in.GetAccessHash() != 0 && stored != nil && stored.GetAccessHash() != 0 && in.GetAccessHash() != stored.GetAccessHash() {
		return nil, mtproto.ErrMediaInvalid
	}
	return UsableStoredDocument(stored, storedErr)
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
