package archive

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

type fakeReader struct {
	folder   int32
	missing  bool
	set      int32
	setN     int
	muted    bool
	contact  bool
	filters  []*mtproto.DialogFilter
	muteN    int
	settings Settings
}

func (f *fakeReader) Folder(context.Context, int64, int32, int64) (int32, bool, error) {
	return f.folder, f.missing, nil
}
func (f *fakeReader) SetFolder(_ context.Context, _ int64, _ int32, _ int64, folder int32) error {
	f.set = folder
	f.setN++
	f.folder = folder
	f.missing = false
	return nil
}
func (f *fakeReader) Settings(context.Context, int64) (Settings, error) {
	return f.settings, nil
}
func (f *fakeReader) Muted(context.Context, int64, int32, int32, int64) (bool, error) {
	return f.muted, nil
}
func (f *fakeReader) MuteForever(context.Context, int64, int32, int64) error {
	f.muteN++
	f.muted = true
	return nil
}
func (f *fakeReader) IsContact(context.Context, int64, int64) (bool, error) {
	return f.contact, nil
}
func (f *fakeReader) CustomFilters(context.Context, int64) ([]*mtproto.DialogFilter, error) {
	return f.filters, nil
}

type memPts struct {
	n    int32
	rows int
}

func (m *memPts) FolderPeers(_ context.Context, _ int64, peers []*mtproto.FolderPeer) (*mtproto.Update, error) {
	m.n++
	m.rows++
	return NewFolderPeersUpdate(peers, m.n), nil
}

func TestApplyLeavesArchivedChat(t *testing.T) {
	reader := &fakeReader{folder: 1, settings: Settings{KeepArchivedUnmuted: true}}
	pts := &memPts{}
	got := Apply(context.Background(), reader, pts, Incoming{
		Owner: 1, PeerType: mtproto.PEER_USER, PeerID: 2, Sender: 2, Class: ClassUser,
	})
	if len(got) != 0 || reader.setN != 0 || pts.rows != 0 {
		t.Fatalf("updates %d writes %d pts %d", len(got), reader.setN, pts.rows)
	}
}

func TestApplyUnarchivesUnmutedIncoming(t *testing.T) {
	reader := &fakeReader{folder: 1}
	pts := &memPts{}
	got := Apply(context.Background(), reader, pts, Incoming{
		Owner: 1, PeerType: mtproto.PEER_CHAT, PeerID: 9, Sender: 2, Class: ClassGroup,
	})
	if len(got) != 1 || got[0].GetPts_INT32() != 1 || got[0].GetPtsCount() != 1 {
		t.Fatalf("update %#v", got)
	}
	if reader.set != 0 || reader.setN != 1 {
		t.Fatalf("folder writes %+v", reader)
	}
	if got[0].GetFolderPeers()[0].GetFolderId() != 0 {
		t.Fatal(got[0].GetFolderPeers()[0].GetFolderId())
	}
}

func TestApplySkipsOutgoingAndMuted(t *testing.T) {
	reader := &fakeReader{folder: 1}
	if ups := Apply(context.Background(), reader, &memPts{}, Incoming{Owner: 1, PeerType: mtproto.PEER_USER, PeerID: 1, Sender: 1}); len(ups) != 0 {
		t.Fatal("outgoing")
	}
	reader.muted = true
	if ups := Apply(context.Background(), reader, &memPts{}, Incoming{Owner: 1, PeerType: mtproto.PEER_USER, PeerID: 2, Sender: 2}); len(ups) != 0 || reader.setN != 0 {
		t.Fatal("muted")
	}
}

func TestApplyArchivesNewNoncontact(t *testing.T) {
	reader := &fakeReader{missing: true, settings: Settings{ArchiveAndMuteNewNoncontactPeers: true}}
	got := Apply(context.Background(), reader, &memPts{}, Incoming{
		Owner: 1, PeerType: mtproto.PEER_USER, PeerID: 2, Sender: 2, Created: true, Class: ClassUser,
	})
	if reader.set != 1 || reader.muteN != 1 {
		t.Fatalf("stored %+v", reader)
	}
	if len(got) != 2 || got[0].GetFolderPeers()[0].GetFolderId() != 1 {
		t.Fatalf("updates %d", len(got))
	}
	if got[1].GetNotifySettings().GetMuteUntil().GetValue() != MuteForever {
		t.Fatal(got[1].GetNotifySettings())
	}
}

func TestApplyKeepsFolderChats(t *testing.T) {
	reader := &fakeReader{
		folder:   1,
		settings: Settings{KeepArchivedFolders: true},
		filters: []*mtproto.DialogFilter{{
			Id:     2,
			Groups: true,
		}},
	}
	got := Apply(context.Background(), reader, &memPts{}, Incoming{
		Owner: 1, PeerType: mtproto.PEER_CHANNEL, PeerID: 4, Sender: 2, Class: ClassGroup,
	})
	if len(got) != 0 || reader.setN != 0 {
		t.Fatal("folder chat was unarchived")
	}
}
