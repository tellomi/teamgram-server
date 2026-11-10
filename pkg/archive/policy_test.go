package archive

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestShouldUnarchiveOfficialRules(t *testing.T) {
	open := Settings{}
	cases := []struct {
		name     string
		folder   int32
		muted    bool
		settings Settings
		inFolder bool
		want     bool
	}{
		{name: "main list stays", folder: 0, want: false},
		{name: "unmuted archived default unarchives", folder: 1, want: true},
		{name: "muted stays", folder: 1, muted: true, want: false},
		{name: "keep unmuted stays", folder: 1, settings: Settings{KeepArchivedUnmuted: true}, want: false},
		{name: "keep folders without a folder still unarchives", folder: 1, settings: Settings{KeepArchivedFolders: true}, want: true},
		{name: "chat in a folder stays when keep folders", folder: 1, settings: Settings{KeepArchivedFolders: true}, inFolder: true, want: false},
		{name: "chat in a folder unarchives when keep folders is off", folder: 1, inFolder: true, want: true},
		{name: "keep unmuted wins over a folder", folder: 1, settings: Settings{KeepArchivedUnmuted: true, KeepArchivedFolders: false}, inFolder: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			settings := tc.settings
			if tc.name == "unmuted archived default unarchives" {
				settings = open
			}
			if got := ShouldUnarchive(tc.folder, tc.muted, settings, tc.inFolder); got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}

func TestShouldArchiveNewPrivate(t *testing.T) {
	on := Settings{ArchiveAndMuteNewNoncontactPeers: true}
	if !ShouldArchiveNewPrivate(true, mtproto.PEER_USER, on, false) {
		t.Fatal("new non-contact private chat")
	}
	if ShouldArchiveNewPrivate(false, mtproto.PEER_USER, on, false) {
		t.Fatal("existing dialog")
	}
	if ShouldArchiveNewPrivate(true, mtproto.PEER_USER, on, true) {
		t.Fatal("contact")
	}
	if ShouldArchiveNewPrivate(true, mtproto.PEER_CHAT, on, false) {
		t.Fatal("group")
	}
	if ShouldArchiveNewPrivate(true, mtproto.PEER_CHANNEL, on, false) {
		t.Fatal("channel")
	}
	if ShouldArchiveNewPrivate(true, mtproto.PEER_USER, Settings{}, false) {
		t.Fatal("setting off")
	}
}

func TestMutedAt(t *testing.T) {
	now := int64(1_700_000_000)
	peerMuted := &mtproto.PeerNotifySettings{MuteUntil: &wrapperspb.Int32Value{Value: MuteForever}}
	peerOpen := &mtproto.PeerNotifySettings{MuteUntil: &wrapperspb.Int32Value{Value: 0}}
	categoryMuted := &mtproto.PeerNotifySettings{MuteUntil: &wrapperspb.Int32Value{Value: int32(now + 60)}}
	if !MutedAt(peerMuted, nil, now) {
		t.Fatal("forever")
	}
	if MutedAt(peerOpen, categoryMuted, now) {
		t.Fatal("explicit unmute ignores the category default")
	}
	if !MutedAt(nil, categoryMuted, now) {
		t.Fatal("category default")
	}
	if MutedAt(nil, &mtproto.PeerNotifySettings{MuteUntil: &wrapperspb.Int32Value{Value: 0}}, now) {
		t.Fatal("category unmute")
	}
	if MutedAt(&mtproto.PeerNotifySettings{MuteUntil: &wrapperspb.Int32Value{Value: int32(now - 10)}}, nil, now) {
		t.Fatal("expired mute")
	}
}

func TestInCustomFolder(t *testing.T) {
	user := mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 7}).To_InputPeer()
	filters := []*mtproto.DialogFilter{
		{PredicateName: mtproto.Predicate_dialogFilterDefault, Id: 0, Contacts: true},
		{Id: 2, IncludePeers: []*mtproto.InputPeer{user}},
		{Id: 3, Groups: true, ExcludePeers: []*mtproto.InputPeer{
			mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 9}).To_InputPeer(),
		}},
	}
	if !InCustomFolder(filters, mtproto.PEER_USER, 7, ClassUser) {
		t.Fatal("included peer")
	}
	if InCustomFolder(filters, mtproto.PEER_USER, 8, ClassContact) {
		t.Fatal("All Chats contacts flag is not a custom folder")
	}
	if !InCustomFolder(filters, mtproto.PEER_CHAT, 4, ClassGroup) {
		t.Fatal("groups flag")
	}
	if InCustomFolder(filters, mtproto.PEER_CHANNEL, 9, ClassGroup) {
		t.Fatal("excluded peer")
	}
	if InCustomFolder(nil, mtproto.PEER_USER, 7, ClassUser) {
		t.Fatal("no folders")
	}
}
