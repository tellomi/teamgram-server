package archive

import (
	"context"
	"time"

	"github.com/teamgram/marmota/pkg/stores/sqlx"
	"github.com/teamgram/proto/mtproto"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// Clients reads archive state through the dialog and user services.
type Clients struct {
	Dialog dialogclient.DialogClient
	User   userclient.UserClient
	Now    func() time.Time
}

func (c Clients) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c Clients) Folder(ctx context.Context, userID int64, peerType int32, peerID int64) (int32, bool, error) {
	if c.Dialog == nil {
		return 0, true, nil
	}
	ext, err := c.Dialog.DialogGetDialogById(ctx, &dialog.TLDialogGetDialogById{
		UserId:   userID,
		PeerType: peerType,
		PeerId:   peerID,
	})
	if err != nil {
		if status.Convert(err).Message() == status.Convert(mtproto.ErrPeerIdInvalid).Message() {
			return 0, true, nil
		}
		return 0, false, err
	}
	if ext == nil || ext.GetDialog() == nil {
		return 0, true, nil
	}
	return ext.GetDialog().GetFolderId().GetValue(), false, nil
}

func (c Clients) SetFolder(ctx context.Context, userID int64, peerType int32, peerID int64, folderID int32) error {
	if c.Dialog == nil {
		return nil
	}
	_, err := c.Dialog.DialogEditPeerFolders(ctx, &dialog.TLDialogEditPeerFolders{
		UserId:         userID,
		PeerDialogList: []int64{mtproto.MakePeerDialogId(peerType, peerID)},
		FolderId:       folderID,
	})
	return err
}

func (c Clients) Settings(ctx context.Context, userID int64) (Settings, error) {
	if c.User == nil {
		return Settings{}, nil
	}
	got, err := c.User.UserGetGlobalPrivacySettings(ctx, &user.TLUserGetGlobalPrivacySettings{UserId: userID})
	if err != nil || got == nil {
		return Settings{}, err
	}
	archiveNew := got.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOLEAN()
	if flag := got.GetArchiveAndMuteNewNoncontactPeers_FLAGBOOL(); flag != nil {
		archiveNew = mtproto.FromBool(flag)
	}
	return Settings{
		ArchiveAndMuteNewNoncontactPeers: archiveNew,
		KeepArchivedUnmuted:              got.GetKeepArchivedUnmuted(),
		KeepArchivedFolders:              got.GetKeepArchivedFolders(),
	}, nil
}

func (c Clients) Muted(ctx context.Context, userID int64, peerType, categoryType int32, peerID int64) (bool, error) {
	if c.User == nil {
		return false, nil
	}
	peer, err := c.User.UserGetNotifySettings(ctx, &user.TLUserGetNotifySettings{
		UserId:   userID,
		PeerType: peerType,
		PeerId:   peerID,
	})
	if err != nil {
		return false, err
	}
	var category *mtproto.PeerNotifySettings
	if categoryType != peerType {
		category, err = c.User.UserGetNotifySettings(ctx, &user.TLUserGetNotifySettings{
			UserId:   userID,
			PeerType: categoryType,
			PeerId:   0,
		})
		if err != nil {
			return false, err
		}
	}
	return MutedAt(peer, category, c.now().Unix()), nil
}

func (c Clients) MuteForever(ctx context.Context, userID int64, peerType int32, peerID int64) error {
	if c.User == nil {
		return nil
	}
	_, err := c.User.UserSetNotifySettings(ctx, &user.TLUserSetNotifySettings{
		UserId:   userID,
		PeerType: peerType,
		PeerId:   peerID,
		Settings: mtproto.MakeTLPeerNotifySettings(&mtproto.PeerNotifySettings{
			ShowPreviews: mtproto.BoolTrue,
			Silent:       mtproto.BoolFalse,
			MuteUntil:    &wrapperspb.Int32Value{Value: MuteForever},
		}).To_PeerNotifySettings(),
	})
	return err
}

func (c Clients) IsContact(ctx context.Context, owner, peer int64) (bool, error) {
	if c.User == nil {
		return false, nil
	}
	got, err := c.User.UserCheckContact(ctx, &user.TLUserCheckContact{UserId: owner, Id: peer})
	if err != nil {
		return false, err
	}
	return mtproto.FromBool(got), nil
}

func (c Clients) CustomFilters(ctx context.Context, userID int64) ([]*mtproto.DialogFilter, error) {
	if c.Dialog == nil {
		return nil, nil
	}
	got, err := c.Dialog.DialogGetDialogFilters(ctx, &dialog.TLDialogGetDialogFilters{UserId: userID})
	if err != nil || got == nil {
		return nil, err
	}
	out := make([]*mtproto.DialogFilter, 0, len(got.GetDatas()))
	for _, row := range got.GetDatas() {
		if row == nil || row.GetDialogFilter() == nil {
			continue
		}
		out = append(out, row.GetDialogFilter())
	}
	return out, nil
}

// SeqPts allocates the next user pts.
type SeqPts interface {
	NextPtsId(ctx context.Context, key int64) int32
}

// SavePts inserts one user_pts_updates row.
type SavePts func(ctx context.Context, userID int64, pts, ptsCount, updateType int32, updateData string) error

// SeqWriter persists updateFolderPeers on the user's pts sequence.
type SeqWriter struct {
	Next SeqPts
	Save SavePts
}

func (w SeqWriter) FolderPeers(ctx context.Context, userID int64, peers []*mtproto.FolderPeer) (*mtproto.Update, error) {
	if w.Next == nil {
		return NewFolderPeersUpdate(peers, 0), nil
	}
	pts := w.Next.NextPtsId(ctx, userID)
	upd := NewFolderPeersUpdate(peers, pts)
	if pts <= 0 || w.Save == nil {
		return upd, nil
	}
	kind, data, err := UpdateData(upd)
	if err != nil {
		return upd, err
	}
	if err := w.Save(ctx, userID, pts, upd.GetPtsCount(), kind, data); err != nil {
		return upd, err
	}
	return upd, nil
}

type ptsRow struct {
	UserID     int64  `db:"user_id"`
	Pts        int32  `db:"pts"`
	PtsCount   int32  `db:"pts_count"`
	UpdateType int32  `db:"update_type"`
	UpdateData string `db:"update_data"`
	Date2      int64  `db:"date2"`
}

// SavePtsSQL inserts user_pts_updates through the shared MySQL pool.
func SavePtsSQL(db *sqlx.DB) SavePts {
	if db == nil {
		return nil
	}
	return func(ctx context.Context, userID int64, pts, ptsCount, updateType int32, updateData string) error {
		_, err := db.NamedExec(ctx,
			"insert into user_pts_updates (user_id, pts, pts_count, update_type, update_data, date2) values (:user_id, :pts, :pts_count, :update_type, :update_data, :date2)",
			&ptsRow{
				UserID:     userID,
				Pts:        pts,
				PtsCount:   ptsCount,
				UpdateType: updateType,
				UpdateData: updateData,
				Date2:      time.Now().Unix(),
			})
		return err
	}
}
