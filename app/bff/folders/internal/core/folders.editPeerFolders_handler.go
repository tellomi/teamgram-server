package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/pkg/archive"
)

// FoldersEditPeerFolders
// folders.editPeerFolders#6847d0ab folder_peers:Vector<InputFolderPeer> = Updates;
// https://core.telegram.org/method/folders.editPeerFolders
//
// folder_id 0 is the main list and 1 is the archive. The result carries
// updateFolderPeers. A stored pts uses pts_count 1; pts 0 keeps pts_count 0
// so the calling client applies the move without a difference gap. Other
// sessions of this user receive the same update. The caller is not pushed
// again.
func (c *FoldersCore) FoldersEditPeerFolders(in *mtproto.TLFoldersEditPeerFolders) (*mtproto.Updates, error) {
	if len(in.GetFolderPeers()) == 0 {
		return mtproto.MakeEmptyUpdates(), nil
	}
	type move struct {
		ref      peerRef
		folderID int32
	}
	moves := make([]move, 0, len(in.GetFolderPeers()))
	for _, fp := range in.GetFolderPeers() {
		if fp == nil {
			return nil, mtproto.ErrPeerIdInvalid
		}
		folderID := fp.GetFolderId()
		if folderID != 0 && folderID != 1 {
			return nil, mtproto.ErrFolderIdInvalid
		}
		ref, err := refFromInput(c.userID(), fp.GetPeer())
		if err != nil {
			return nil, err
		}
		if ref.Kind == "user" && ref.ID == c.userID() {
			return nil, mtproto.ErrPeerIdInvalid
		}
		moves = append(moves, move{ref: ref, folderID: folderID})
	}
	// Last assignment wins when the same peer is listed twice.
	byPeer := map[string]move{}
	order := make([]string, 0, len(moves))
	for _, mv := range moves {
		if _, ok := byPeer[mv.ref.key()]; !ok {
			order = append(order, mv.ref.key())
		}
		byPeer[mv.ref.key()] = mv
	}
	byFolder := map[int32][]int64{}
	var folderPeers []*mtproto.FolderPeer
	for _, key := range order {
		mv := byPeer[key]
		// Pin and archive use the same dialog row. A row with top_message 0
		// is still a dialog; getDialogsByIdList's placeholder is not.
		if err := c.requireStoredDialog(mv.ref); err != nil {
			return nil, err
		}
		byFolder[mv.folderID] = append(byFolder[mv.folderID], mv.ref.dialogID())
		folderPeers = append(folderPeers, mtproto.MakeTLFolderPeer(&mtproto.FolderPeer{
			Peer:     mv.ref.peer(),
			FolderId: mv.folderID,
		}).To_FolderPeer())
	}
	for _, folderID := range []int32{0, 1} {
		list := byFolder[folderID]
		if len(list) == 0 {
			continue
		}
		if _, err := c.svcCtx.Dao.DialogClient.DialogEditPeerFolders(c.ctx, &dialog.TLDialogEditPeerFolders{
			UserId:         c.userID(),
			PeerDialogList: list,
			FolderId:       folderID,
		}); err != nil {
			c.Logger.Errorf("folders.editPeerFolders - error: %v", err)
			return nil, err
		}
	}
	upd := folderPeerUpdates(folderPeers, c.allocFolderPts(folderPeers))
	c.pushFolderPeers(upd)
	return upd, nil
}

// requireStoredDialog accepts a dialog row that messages.toggleDialogPin can
// already see. getDialogsByIdList invents a top_message 0 row for a missing
// peer, and treating that as "not a dialog" made archive return
// PEER_ID_INVALID for a user who had just been pinned.
func (c *FoldersCore) requireStoredDialog(ref peerRef) error {
	denied := mtproto.ErrPeerIdInvalid
	switch ref.Kind {
	case "channel":
		denied = mtproto.ErrChannelPrivate
	case "chat":
		denied = mtproto.ErrChatIdInvalid
	}
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.DialogClient == nil {
		return denied
	}
	ext, err := c.svcCtx.Dao.DialogClient.DialogGetDialogById(c.ctx, &dialog.TLDialogGetDialogById{
		UserId:   c.userID(),
		PeerType: ref.peerType(),
		PeerId:   ref.ID,
	})
	if err != nil {
		if sameRPC(err, mtproto.ErrPeerIdInvalid) {
			return denied
		}
		c.Logger.Errorf("folders.editPeerFolders - dialog: %v", err)
		return err
	}
	if ext == nil || ext.GetDialog() == nil {
		return denied
	}
	return nil
}

// pushFolderPeers delivers updateFolderPeers to the account's other sessions.
// The RPC result is what the calling session applies. A second push to the
// same user would deliver the update to that session again.
func (c *FoldersCore) pushFolderPeers(upd *mtproto.Updates) {
	if upd == nil || len(upd.GetUpdates()) != 1 {
		return
	}
	c.pushNotMe(upd.GetUpdates()[0])
}

// allocFolderPts stores updateFolderPeers on the user's pts sequence and
// returns that pts. Zero means the process has no idgen or MySQL client;
// the update is still pushed, with pts_count 0.
func (c *FoldersCore) allocFolderPts(peers []*mtproto.FolderPeer) int32 {
	if c == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.NextPts == nil || c.userID() == 0 {
		return 0
	}
	writer := archive.SeqWriter{Next: ptsFunc(c.svcCtx.Dao.NextPts), Save: c.svcCtx.Dao.SavePts}
	upd, err := writer.FolderPeers(c.ctx, c.userID(), peers)
	if err != nil {
		c.Logger.Errorf("folders.editPeerFolders - pts: %v", err)
	}
	if upd == nil {
		return 0
	}
	return upd.GetPts_INT32()
}

type ptsFunc func(ctx context.Context, userID int64) int32

func (f ptsFunc) NextPtsId(ctx context.Context, key int64) int32 {
	if f == nil {
		return 0
	}
	return f(ctx, key)
}
