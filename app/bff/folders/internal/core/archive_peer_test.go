package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/folders/internal/svc"
	syncclient "github.com/teamgram/teamgram-server/app/messenger/sync/client"
	syncpb "github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/zeromicro/go-zero/core/logx"
	"google.golang.org/grpc/status"
)

type archiveDialog struct {
	dialogclient.DialogClient
	row    *dialog.DialogExt
	err    error
	folder int32
	calls  int
}

func (a *archiveDialog) DialogGetDialogById(context.Context, *dialog.TLDialogGetDialogById) (*dialog.DialogExt, error) {
	if a.err != nil {
		return nil, a.err
	}
	return a.row, nil
}

func (a *archiveDialog) DialogEditPeerFolders(_ context.Context, in *dialog.TLDialogEditPeerFolders) (*dialog.Vector_DialogPinnedExt, error) {
	a.calls++
	a.folder = in.GetFolderId()
	return &dialog.Vector_DialogPinnedExt{}, nil
}

type archiveSync struct {
	syncclient.SyncClient
	notMe  int
	pushes []int64
}

func (s *archiveSync) SyncUpdatesNotMe(context.Context, *syncpb.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	s.notMe++
	return mtproto.EmptyVoid, nil
}

func (s *archiveSync) SyncPushUpdates(_ context.Context, in *syncpb.TLSyncPushUpdates) (*mtproto.Void, error) {
	s.pushes = append(s.pushes, in.GetUserId())
	return mtproto.EmptyVoid, nil
}

func archiveCore(user int64, rows *archiveDialog, syncer *archiveSync) *FoldersCore {
	return &FoldersCore{
		ctx:    context.Background(),
		MD:     &metadata.RpcMetadata{UserId: user, PermAuthKeyId: 99},
		Logger: logx.WithContext(context.Background()),
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			DialogClient: rows,
			SyncClient:   syncer,
		}},
	}
}

func userFolder(id, folder int64) *mtproto.TLFoldersEditPeerFolders {
	return &mtproto.TLFoldersEditPeerFolders{
		FolderPeers: []*mtproto.InputFolderPeer{
			mtproto.MakeTLInputFolderPeer(&mtproto.InputFolderPeer{
				Peer:     mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: id, AccessHash: 5}).To_InputPeer(),
				FolderId: int32(folder),
			}).To_InputFolderPeer(),
		},
	}
}

func TestArchiveUserWithEmptyTopMessage(t *testing.T) {
	row := dialog.MakeTLDialogExt(&dialog.DialogExt{
		Date: 20,
		Dialog: mtproto.MakeTLDialog(&mtproto.Dialog{
			Peer:       mtproto.MakePeerUser(9),
			TopMessage: 0,
		}).To_Dialog(),
	}).To_DialogExt()
	store := &archiveDialog{row: row}
	syncer := &archiveSync{}
	c := archiveCore(7, store, syncer)
	up, err := c.FoldersEditPeerFolders(userFolder(9, 1))
	if err != nil {
		t.Fatal(err)
	}
	if store.calls != 1 || store.folder != 1 {
		t.Fatalf("stored folder %d calls %d", store.folder, store.calls)
	}
	if len(up.GetUpdates()) != 1 || up.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateFolderPeers {
		t.Fatalf("updates %#v", up.GetUpdates())
	}
	if folderOf(up.GetUpdates()[0], 9) != 1 {
		t.Fatal("folder id")
	}
	if syncer.notMe != 1 || len(syncer.pushes) != 0 {
		t.Fatalf("other sessions only: notMe=%d pushes=%v", syncer.notMe, syncer.pushes)
	}

	if _, err := c.FoldersEditPeerFolders(userFolder(9, 0)); err != nil {
		t.Fatal(err)
	}
	if store.folder != 0 {
		t.Fatalf("unarchive folder %d", store.folder)
	}
}

func TestArchiveMissingPeers(t *testing.T) {
	store := &archiveDialog{err: mtproto.ErrPeerIdInvalid}
	c := archiveCore(7, store, &archiveSync{})
	if _, err := c.FoldersEditPeerFolders(userFolder(9, 1)); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatalf("user: %v", err)
	}
	if _, err := c.FoldersEditPeerFolders(&mtproto.TLFoldersEditPeerFolders{
		FolderPeers: []*mtproto.InputFolderPeer{
			mtproto.MakeTLInputFolderPeer(&mtproto.InputFolderPeer{
				Peer:     mtproto.MakeTLInputPeerChat(&mtproto.InputPeer{ChatId: 4}).To_InputPeer(),
				FolderId: 1,
			}).To_InputFolderPeer(),
		},
	}); status.Convert(err).Message() != "CHAT_ID_INVALID" {
		t.Fatalf("chat: %v", err)
	}
	if _, err := c.FoldersEditPeerFolders(&mtproto.TLFoldersEditPeerFolders{
		FolderPeers: []*mtproto.InputFolderPeer{
			mtproto.MakeTLInputFolderPeer(&mtproto.InputFolderPeer{
				Peer:     mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 2000000000}).To_InputPeer(),
				FolderId: 1,
			}).To_InputFolderPeer(),
		},
	}); status.Convert(err).Message() != "CHANNEL_PRIVATE" {
		t.Fatalf("channel: %v", err)
	}
	if _, err := c.FoldersEditPeerFolders(userFolder(7, 1)); status.Convert(err).Message() != "PEER_ID_INVALID" {
		t.Fatalf("self: %v", err)
	}
	if store.calls != 0 {
		t.Fatal("missing dialog was written")
	}
}

func folderOf(upd *mtproto.Update, user int64) int32 {
	for _, fp := range upd.GetFolderPeers() {
		if fp.GetPeer().GetUserId() == user {
			return fp.GetFolderId()
		}
	}
	return -1
}
