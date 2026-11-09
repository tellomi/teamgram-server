package archive

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/zeromicro/go-zero/core/jsonx"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Incoming is one new message delivered to Owner.
// Created is true only when Owner had no dialog with the peer before this message.
type Incoming struct {
	Owner    int64
	PeerType int32
	PeerID   int64
	Sender   int64
	Class    PeerClass
	Created  bool
}

// FolderReader loads the current folder and privacy inputs.
type FolderReader interface {
	// Folder returns the stored folder id. missing is true when the dialog row
	// does not exist. A transport error is returned as err and must not be
	// treated as a missing dialog.
	Folder(ctx context.Context, userID int64, peerType int32, peerID int64) (folderID int32, missing bool, err error)
	SetFolder(ctx context.Context, userID int64, peerType int32, peerID int64, folderID int32) error
	Settings(ctx context.Context, userID int64) (Settings, error)
	Muted(ctx context.Context, userID int64, peerType, categoryType int32, peerID int64) (bool, error)
	MuteForever(ctx context.Context, userID int64, peerType int32, peerID int64) error
	IsContact(ctx context.Context, owner, peer int64) (bool, error)
	CustomFilters(ctx context.Context, userID int64) ([]*mtproto.DialogFilter, error)
}

// PtsWriter allocates one pts, stores updateFolderPeers, and returns the update
// the client must see. pts is 0 when the writer cannot persist a sequence.
type PtsWriter interface {
	FolderPeers(ctx context.Context, userID int64, peers []*mtproto.FolderPeer) (*mtproto.Update, error)
}

// Apply applies official archive rules for one incoming message.
// The returned updates are updateFolderPeers (with pts) and, for a new
// non-contact under archive_and_mute_new_noncontact_peers, updateNotifySettings.
// The caller delivers them on the same push as the new message.
func Apply(ctx context.Context, reader FolderReader, pts PtsWriter, in Incoming) []*mtproto.Update {
	if reader == nil || in.Owner == 0 || in.PeerID == 0 || in.Owner == in.Sender {
		return nil
	}
	settings, err := reader.Settings(ctx, in.Owner)
	if err != nil {
		logx.WithContext(ctx).Errorf("archive.settings user=%d err=%v", in.Owner, err)
		settings = Settings{}
	}

	if ShouldArchiveNewPrivate(in.Created, in.PeerType, settings, false) {
		contact, err := reader.IsContact(ctx, in.Owner, in.PeerID)
		if err != nil {
			logx.WithContext(ctx).Errorf("archive.contact user=%d peer=%d err=%v", in.Owner, in.PeerID, err)
			contact = true
		}
		if ShouldArchiveNewPrivate(in.Created, in.PeerType, settings, contact) {
			return archiveNew(ctx, reader, pts, in)
		}
	}

	folder, missing, err := reader.Folder(ctx, in.Owner, in.PeerType, in.PeerID)
	if err != nil || missing || folder != 1 {
		return nil
	}
	muted, err := reader.Muted(ctx, in.Owner, in.PeerType, CategoryPeerType(in.PeerType, in.Class), in.PeerID)
	if err != nil {
		logx.WithContext(ctx).Errorf("archive.mute user=%d peer=%d err=%v", in.Owner, in.PeerID, err)
		return nil
	}
	inFolder := false
	if settings.KeepArchivedFolders {
		filters, err := reader.CustomFilters(ctx, in.Owner)
		if err != nil {
			logx.WithContext(ctx).Errorf("archive.filters user=%d err=%v", in.Owner, err)
			return nil
		}
		inFolder = InCustomFolder(filters, in.PeerType, in.PeerID, in.Class)
	}
	if !ShouldUnarchive(folder, muted, settings, inFolder) {
		return nil
	}
	if err := reader.SetFolder(ctx, in.Owner, in.PeerType, in.PeerID, 0); err != nil {
		logx.WithContext(ctx).Errorf("archive.unarchive user=%d peer=%d err=%v", in.Owner, in.PeerID, err)
		return nil
	}
	upd := folderUpdate(ctx, pts, in.Owner, in.PeerType, in.PeerID, 0)
	if upd == nil {
		return nil
	}
	return []*mtproto.Update{upd}
}

func archiveNew(ctx context.Context, reader FolderReader, pts PtsWriter, in Incoming) []*mtproto.Update {
	if err := reader.SetFolder(ctx, in.Owner, in.PeerType, in.PeerID, 1); err != nil {
		logx.WithContext(ctx).Errorf("archive.new user=%d peer=%d err=%v", in.Owner, in.PeerID, err)
		return nil
	}
	if err := reader.MuteForever(ctx, in.Owner, in.PeerType, in.PeerID); err != nil {
		logx.WithContext(ctx).Errorf("archive.mute-new user=%d peer=%d err=%v", in.Owner, in.PeerID, err)
	}
	var out []*mtproto.Update
	if upd := folderUpdate(ctx, pts, in.Owner, in.PeerType, in.PeerID, 1); upd != nil {
		out = append(out, upd)
	}
	out = append(out, mtproto.MakeTLUpdateNotifySettings(&mtproto.Update{
		Peer_NOTIFYPEER: mtproto.MakePeerUtil(in.PeerType, in.PeerID).ToNotifyPeer(),
		NotifySettings: mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
			ShowPreviews: mtproto.BoolTrue,
			Silent:       mtproto.BoolFalse,
			MuteUntil:    &wrapperspb.Int32Value{Value: MuteForever},
		}).To_PeerNotifySettings(),
	}).To_Update())
	return out
}

func folderUpdate(ctx context.Context, pts PtsWriter, userID int64, peerType int32, peerID int64, folderID int32) *mtproto.Update {
	peers := []*mtproto.FolderPeer{mtproto.MakeTLFolderPeer(&mtproto.FolderPeer{
		Peer:     mtproto.MakePeer(peerType, peerID),
		FolderId: folderID,
	}).To_FolderPeer()}
	if pts == nil {
		return NewFolderPeersUpdate(peers, 0)
	}
	upd, err := pts.FolderPeers(ctx, userID, peers)
	if err != nil {
		logx.WithContext(ctx).Errorf("archive.pts user=%d err=%v", userID, err)
	}
	if upd != nil {
		return upd
	}
	return NewFolderPeersUpdate(peers, 0)
}

// NewFolderPeersUpdate builds updateFolderPeers. pts 0 leaves pts_count 0 so a
// client applies the folder move without opening a difference gap. A persisted
// update uses pts_count 1, one step in the user's pts sequence.
func NewFolderPeersUpdate(peers []*mtproto.FolderPeer, pts int32) *mtproto.Update {
	if peers == nil {
		peers = []*mtproto.FolderPeer{}
	}
	count := int32(0)
	if pts > 0 {
		count = 1
	}
	return mtproto.MakeTLUpdateFolderPeers(&mtproto.Update{
		FolderPeers: peers,
		Pts_INT32:   pts,
		PtsCount:    count,
	}).To_Update()
}

// UpdateData is the JSON stored in user_pts_updates.update_data.
func UpdateData(update *mtproto.Update) (updateType int32, data string, err error) {
	if update != nil {
		update.SanitizeEmbeddedEntities()
	}
	raw, err := jsonx.Marshal(update)
	if err != nil {
		return 0, "", err
	}
	return mtproto.GetUpdateType(update), string(raw), nil
}
