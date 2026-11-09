package core

import (
	"context"
	"sync"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/dao"
	"github.com/teamgram/teamgram-server/app/bff/dialogs/internal/svc"
	dialogclient "github.com/teamgram/teamgram-server/app/service/biz/dialog/client"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	updatesclient "github.com/teamgram/teamgram-server/app/service/biz/updates/client"
	"github.com/teamgram/teamgram-server/app/service/biz/updates/updates"
	"github.com/zeromicro/go-zero/core/logx"
)

// A message that lands while pinned dialogs are loaded must stay visible to
// updates.getDifference. That holds when state.pts is taken before the rows,
// so the returned pts is not already past that message.
func TestPinnedStateIsTakenBeforeDialogs(t *testing.T) {
	seq := &pinnedSeq{pts: 100}
	c := newPinnedCore(seq)
	got, err := c.MessagesGetPinnedDialogs(&mtproto.TLMessagesGetPinnedDialogs{})
	if err != nil {
		t.Fatal(err)
	}
	if seq.dialogsBeforeState {
		t.Fatal("dialog rows were loaded before updates state")
	}
	if got.GetState() == nil || got.GetState().GetPts() != 100 {
		t.Fatalf("state %#v", got.GetState())
	}
	// Both dialog loads observe a write after the state was captured.
	if seq.pts <= 100 {
		t.Fatalf("pts after the dialog load %d", seq.pts)
	}
	if seq.folderCalls != 1 || seq.pinnedCalls != 1 {
		t.Fatalf("folder %d pinned %d", seq.folderCalls, seq.pinnedCalls)
	}
}

func TestPinnedDialogsStopWhenStateFails(t *testing.T) {
	seq := &pinnedSeq{stateErr: mtproto.ErrInternalServerError}
	c := newPinnedCore(seq)
	if _, err := c.MessagesGetPinnedDialogs(&mtproto.TLMessagesGetPinnedDialogs{}); err == nil {
		t.Fatal("state failure was ignored")
	}
	if seq.folderCalls != 0 || seq.pinnedCalls != 0 {
		t.Fatal("dialogs were loaded after state failed")
	}
}

func newPinnedCore(seq *pinnedSeq) *DialogsCore {
	ctx := context.Background()
	return &DialogsCore{
		ctx: ctx,
		svcCtx: &svc.ServiceContext{Dao: &dao.Dao{
			UpdatesClient: seq,
			DialogClient:  seq,
		}},
		Logger: logx.WithContext(ctx),
		MD:     &metadata.RpcMetadata{UserId: 7, PermAuthKeyId: 9},
	}
}

type pinnedSeq struct {
	updatesclient.UpdatesClient
	dialogclient.DialogClient

	mu                 sync.Mutex
	pts                int32
	stateDone          bool
	dialogsBeforeState bool
	stateErr           error
	folderCalls        int
	pinnedCalls        int
}

func (s *pinnedSeq) UpdatesGetStateV2(context.Context, *updates.TLUpdatesGetStateV2) (*mtproto.Updates_State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stateErr != nil {
		return nil, s.stateErr
	}
	s.stateDone = true
	return &mtproto.Updates_State{Pts: s.pts, Qts: 1, Date: 1, Seq: 1}, nil
}

func (s *pinnedSeq) noteDialog() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.stateDone {
		s.dialogsBeforeState = true
	}
	// The write is newer than the state the handler already captured.
	s.pts++
}

func (s *pinnedSeq) DialogGetDialogFolder(context.Context, *dialog.TLDialogGetDialogFolder) (*dialog.Vector_DialogExt, error) {
	s.folderCalls++
	s.noteDialog()
	return &dialog.Vector_DialogExt{}, nil
}

func (s *pinnedSeq) DialogGetPinnedDialogs(context.Context, *dialog.TLDialogGetPinnedDialogs) (*dialog.Vector_DialogExt, error) {
	s.pinnedCalls++
	s.noteDialog()
	return &dialog.Vector_DialogExt{}, nil
}
