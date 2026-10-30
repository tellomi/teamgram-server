package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
)

// FoldersEditPeerFolders
// folders.editPeerFolders#6847d0ab folder_peers:Vector<InputFolderPeer> = Updates;
// https://core.telegram.org/method/folders.editPeerFolders
//
// folder_id 0 is the main list and 1 is the archive. The result carries
// updateFolderPeers with pts_count 0 so the calling client applies it without
// a difference gap. Other sessions receive the same update through sync.
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
	ids := make([]int64, 0, len(order))
	for _, key := range order {
		ids = append(ids, byPeer[key].ref.dialogID())
	}
	present, err := c.dialogsPresent(ids)
	if err != nil {
		c.Logger.Errorf("folders.editPeerFolders - dialogs: %v", err)
		return nil, err
	}
	byFolder := map[int32][]int64{}
	var folderPeers []*mtproto.FolderPeer
	for _, key := range order {
		mv := byPeer[key]
		if _, ok := present[mv.ref.dialogID()]; !ok {
			switch mv.ref.Kind {
			case "channel":
				return nil, mtproto.ErrChannelPrivate
			case "chat":
				return nil, mtproto.ErrChatIdInvalid
			default:
				return nil, mtproto.ErrPeerIdInvalid
			}
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
	upd := folderPeerUpdates(folderPeers)
	if len(upd.GetUpdates()) == 1 {
		c.pushNotMe(upd.GetUpdates()[0])
	}
	return upd, nil
}
