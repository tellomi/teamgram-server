package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountassets"
	"github.com/teamgram/teamgram-server/app/bff/stickers/catalog"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/story"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/app/service/media/media"
	"github.com/teamgram/teamgram-server/pkg/mediaaccess"
)

// authorizeDownload is set by tests. Production leaves it nil and checks
// the location. Tests that stub downloadFile and do not set this skip the
// check, because they have no media service.
var authorizeDownload func(c *FilesCore, location *mtproto.InputFileLocation) error

// authorize runs before any byte of the file is read, including a CDN token.
func (c *FilesCore) authorize(location *mtproto.InputFileLocation) error {
	var err error
	switch {
	case authorizeDownload != nil:
		err = authorizeDownload(c, location)
	case downloadFile != nil:
		return nil
	default:
		err = c.authorizeLocation(location)
	}
	var log = c.Logger
	if c == nil {
		log = nil
	}
	return mediaaccess.Gate(log, err)
}

// authorizeLocation checks one InputFileLocation.
//
// Photo and document locations must present the access_hash stored for that
// file. Stickers, emoji, reactions, and panel GIFs use the catalog hash,
// which the set returns to every caller. Story, channel, and forwarded media
// use the same photo or document hash the viewer was given. Peer photos have
// no file hash: the peer must be visible and the photo must be that peer's.
// Secure files are not stored here. file_reference is checked only when a
// non-empty one was stored.
func (c *FilesCore) authorizeLocation(location *mtproto.InputFileLocation) error {
	if location == nil {
		return mtproto.ErrLocationInvalid
	}
	switch location.GetPredicateName() {
	case mtproto.Predicate_inputDocumentFileLocation:
		return c.authorizeDocument(location)
	case mtproto.Predicate_inputPhotoFileLocation, mtproto.Predicate_inputPhotoLegacyFileLocation:
		return c.authorizePhoto(location)
	case mtproto.Predicate_inputPeerPhotoFileLocation:
		return c.authorizePeerPhoto(location)
	case mtproto.Predicate_inputEncryptedFileLocation:
		return c.authorizeEncrypted(location)
	case mtproto.Predicate_inputSecureFileLocation:
		return mtproto.ErrFileIdInvalid
	case mtproto.Predicate_inputTakeoutFileLocation, mtproto.Predicate_inputFileLocation:
		return mtproto.ErrInputRequestInvalid
	case mtproto.Predicate_inputGroupCallStream, mtproto.Predicate_inputStickerSetThumb:
		return nil
	default:
		return mtproto.ErrLocationInvalid
	}
}

func (c *FilesCore) authorizeDocument(location *mtproto.InputFileLocation) error {
	id := location.GetId()
	if hashes, ref, ok := catalog.ReactionIssued(id); ok {
		if !issuedHash(location.GetAccessHash(), hashes) {
			return mtproto.ErrFileIdInvalid
		}
		return mediaaccess.Reference(location.GetFileReference(), ref)
	}
	if hash, _, ok := catalog.Must().DocumentAccess(id); ok {
		return mediaaccess.DocumentDownload(id, location.GetAccessHash(), location.GetFileReference(), true, hash, nil)
	}
	if hash, ok := catalog.GifAccess(id); ok {
		return mediaaccess.DocumentDownload(id, location.GetAccessHash(), location.GetFileReference(), true, hash, nil)
	}
	if hash, ok := accountassets.DocumentAccess(id); ok {
		return mediaaccess.DocumentDownload(id, location.GetAccessHash(), location.GetFileReference(), true, hash, nil)
	}
	doc, err := c.loadDocument(id)
	if err != nil {
		return err
	}
	found := doc != nil && doc.GetPredicateName() == mtproto.Predicate_document && doc.GetId() != 0
	var stored int64
	var ref []byte
	if doc != nil {
		stored = doc.GetAccessHash()
		ref = doc.GetFileReference()
	}
	if err := mediaaccess.DocumentDownload(id, location.GetAccessHash(), location.GetFileReference(), found, stored, ref); err != nil {
		return err
	}
	return c.storyMedia(id)
}

func issuedHash(got int64, hashes []int64) bool {
	for _, hash := range hashes {
		if mediaaccess.SameHash(got, hash) {
			return true
		}
	}
	return false
}

func (c *FilesCore) authorizePhoto(location *mtproto.InputFileLocation) error {
	id := location.GetId()
	if _, _, ok := chatbox.WebPhotoForDownload(id, location.GetAccessHash()); ok {
		return mediaaccess.PhotoDownload(id, location.GetAccessHash(), location.GetFileReference(), false, 0, nil, true)
	}
	photo, err := c.loadPhoto(id)
	if err != nil {
		return err
	}
	found := photo != nil && photo.GetPredicateName() == mtproto.Predicate_photo && photo.GetId() != 0
	var stored int64
	var ref []byte
	if photo != nil {
		stored = photo.GetAccessHash()
		ref = photo.GetFileReference()
	}
	if err := mediaaccess.PhotoDownload(id, location.GetAccessHash(), location.GetFileReference(), found, stored, ref, false); err != nil {
		return err
	}
	return c.storyMedia(id)
}

func (c *FilesCore) storyMedia(id int64) error {
	if c == nil {
		return nil
	}
	return story.Shared().AllowMediaDownload(c.ctx, c.userID(), id)
}

func (c *FilesCore) authorizeEncrypted(location *mtproto.InputFileLocation) error {
	store := c.accessStore()
	if store == nil {
		return mtproto.ErrFileIdInvalid
	}
	hash, found, err := store.EncryptedAccess(c.ctx, location.GetId())
	if err != nil {
		return err
	}
	return mediaaccess.EncryptedDownload(location.GetId(), location.GetAccessHash(), hash, found)
}

func (c *FilesCore) authorizePeerPhoto(location *mtproto.InputFileLocation) error {
	peer := location.GetPeer()
	photoID := location.GetPhotoId()
	if peer == nil {
		return mtproto.ErrLocationInvalid
	}
	switch peer.GetPredicateName() {
	case mtproto.Predicate_inputPeerSelf:
		ids, err := c.profilePhotoIDs(c.userID())
		if err != nil {
			return err
		}
		return mediaaccess.ProfilePhoto(photoID, ids, true)
	case mtproto.Predicate_inputPeerUser, mtproto.Predicate_inputPeerUserFromMessage:
		if err := c.peerUserOK(peer.GetUserId(), peer.GetAccessHash()); err != nil {
			return err
		}
		ids, err := c.profilePhotoIDs(peer.GetUserId())
		if err != nil {
			return err
		}
		return mediaaccess.ProfilePhoto(photoID, ids, true)
	case mtproto.Predicate_inputPeerChat:
		store := c.accessStore()
		if store == nil {
			return mtproto.ErrPeerIdInvalid
		}
		chatPhoto, member, err := store.ChatPhoto(c.ctx, c.userID(), peer.GetChatId())
		if err != nil {
			return err
		}
		return mediaaccess.ChatPhoto(photoID, chatPhoto, member)
	case mtproto.Predicate_inputPeerChannel, mtproto.Predicate_inputPeerChannelFromMessage:
		store := c.accessStore()
		if store == nil {
			return mtproto.ErrChannelInvalid
		}
		channelPhoto, hash, found, err := store.ChannelPhoto(c.ctx, peer.GetChannelId())
		if err != nil {
			return err
		}
		return mediaaccess.ChannelPhoto(photoID, channelPhoto, peer.GetAccessHash(), hash, found)
	default:
		return mtproto.ErrPeerIdInvalid
	}
}

func (c *FilesCore) loadDocument(id int64) (*mtproto.Document, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MediaClient == nil {
		return nil, mtproto.ErrFileIdInvalid
	}
	return c.svcCtx.Dao.MediaGetDocument(c.ctx, &media.TLMediaGetDocument{Id: id})
}

func (c *FilesCore) loadPhoto(id int64) (*mtproto.Photo, error) {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.MediaClient == nil {
		return nil, mtproto.ErrFileIdInvalid
	}
	return c.svcCtx.Dao.MediaGetPhoto(c.ctx, &media.TLMediaGetPhoto{PhotoId: id})
}

func (c *FilesCore) userID() int64 {
	if c == nil || c.MD == nil {
		return 0
	}
	return c.MD.UserId
}

func (c *FilesCore) accessStore() *mediaaccess.Store {
	if c == nil || c.svcCtx == nil {
		return nil
	}
	return c.svcCtx.Access
}

func (c *FilesCore) peerUserOK(userID, access int64) error {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil || userID == 0 {
		return mtproto.ErrPeerIdInvalid
	}
	user, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil {
		return err
	}
	if user == nil || user.Id() == 0 || user.Deleted() || !mediaaccess.SameHash(access, user.AccessHash()) {
		return mtproto.ErrPeerIdInvalid
	}
	return nil
}

func (c *FilesCore) profilePhotoIDs(userID int64) ([]int64, error) {
	if userID == 0 || c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return nil, mtproto.ErrPeerIdInvalid
	}
	listed, err := c.svcCtx.Dao.UserGetProfilePhotos(c.ctx, &userpb.TLUserGetProfilePhotos{UserId: userID})
	if err != nil {
		return nil, err
	}
	var ids []int64
	if listed != nil {
		ids = append(ids, listed.GetDatas()...)
	}
	user, err := c.svcCtx.Dao.UserGetImmutableUser(c.ctx, &userpb.TLUserGetImmutableUser{Id: userID})
	if err != nil {
		return ids, nil
	}
	if user != nil {
		if p := user.ProfilePhoto(); p != nil && p.GetPhotoId() != 0 {
			ids = append(ids, p.GetPhotoId())
		}
		if p := user.Photo(); p != nil && p.GetId() != 0 {
			ids = append(ids, p.GetId())
		}
	}
	return ids, nil
}

// documentReadable is the getDocumentByHash allow-list: a public sticker,
// emoji, reaction, or panel GIF, or a document this user already has.
func (c *FilesCore) documentReadable(doc *mtproto.Document) (bool, error) {
	if doc == nil || doc.GetId() == 0 {
		return false, nil
	}
	if hash, public, ok := catalog.Must().DocumentAccess(doc.GetId()); ok && public && mediaaccess.SameHash(hash, doc.GetAccessHash()) {
		return true, nil
	}
	if hash, ok := catalog.GifAccess(doc.GetId()); ok && mediaaccess.SameHash(hash, doc.GetAccessHash()) {
		return true, nil
	}
	store := c.accessStore()
	if store == nil {
		return false, nil
	}
	return store.UserSeesDocument(c.ctx, c.userID(), doc.GetId())
}
