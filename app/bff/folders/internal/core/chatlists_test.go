package core

import (
	"bytes"
	"strings"
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestParseInviteSlug(t *testing.T) {
	owner, id, ok := parseSlug("c42_7_0123456789abcdef")
	if !ok || owner != 42 || id != 7 {
		t.Fatalf("parsed %d %d %v", owner, id, ok)
	}
	if _, _, ok := parseSlug(""); ok {
		t.Fatal("empty slug")
	}
	if _, _, ok := parseSlug("addlist"); ok {
		t.Fatal("plain slug")
	}
	slug, err := newInviteSlug(9, 3)
	if err != nil {
		t.Fatal(err)
	}
	gotOwner, gotID, ok := parseSlug(slug)
	if !ok || gotOwner != 9 || gotID != 3 {
		t.Fatalf("roundtrip %s -> %d %d %v", slug, gotOwner, gotID, ok)
	}
	if !strings.HasPrefix(inviteURL(slug), "https://t.me/addlist/c9_3_") {
		t.Fatal(inviteURL(slug))
	}
}

func TestChatlistBlobDoesNotBecomeTitle(t *testing.T) {
	blob := encodeInvites([]storedInvite{{
		Slug:  "c1_2_0123456789abcdef",
		Title: "Link",
		Date:  10,
		Peers: []peerRef{{Kind: "channel", ID: 5}},
	}})
	filter := &mtproto.DialogFilter{
		PredicateName: mtproto.Predicate_dialogFilterChatlist,
		Id:            2,
		Title:         "News",
		Title_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
			Text:     "News",
			Entities: []*mtproto.MessageEntity{},
		}).To_TextWithEntities(),
		Title_STRING: blob,
		HasMyInvites: true,
	}
	filter.FixData()
	if filter.GetTitle() != "News" {
		t.Fatalf("title %q", filter.GetTitle())
	}
	if filter.GetTitle_TEXTWITHENTITIES().GetText() != "News" {
		t.Fatal("entities title")
	}
	if !strings.HasPrefix(filter.GetTitle_STRING(), mtproto.ChatlistInvitesBlobPrefix) {
		t.Fatal("blob dropped")
	}
	if mtproto.ChatlistInviteCount(filter.GetTitle_STRING()) != 1 {
		t.Fatal("count")
	}

	client := clientDialogFilter(filter)
	if strings.Contains(client.GetTitle_STRING(), "chatlist-invites") {
		t.Fatalf("client title %q", client.GetTitle_STRING())
	}
	if !strings.HasPrefix(filter.GetTitle_STRING(), mtproto.ChatlistInvitesBlobPrefix) {
		t.Fatal("strip mutated the stored filter")
	}
	buf := mtproto.NewEncodeBuf(256)
	if err := client.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.GetBuf(), []byte("chatlist-invites")) {
		t.Fatal("blob leaked into layer 232")
	}
	buf = mtproto.NewEncodeBuf(256)
	if err := client.Encode(buf, 195); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(buf.GetBuf(), []byte("chatlist-invites")) {
		t.Fatal("blob leaked into legacy layer")
	}
}

func TestPreserveChatlistBlob(t *testing.T) {
	stored := &mtproto.DialogFilter{Title_STRING: encodeInvites([]storedInvite{{Slug: "c1_2_0123456789abcdef", Title: "L"}})}
	incoming := &mtproto.DialogFilter{Title: "News", HasMyInvites: false}
	mtproto.PreserveChatlistBlob(incoming, stored)
	if mtproto.ChatlistInviteCount(incoming.GetTitle_STRING()) != 1 || !incoming.GetHasMyInvites() {
		t.Fatalf("preserved %+v", incoming)
	}
	replacement := encodeInvites(nil)
	cleared := &mtproto.DialogFilter{Title_STRING: replacement}
	mtproto.PreserveChatlistBlob(cleared, stored)
	if cleared.GetTitle_STRING() != replacement || cleared.GetHasMyInvites() {
		t.Fatal("explicit blob should replace stored invites")
	}
}

func TestChatlistResultsEncode(t *testing.T) {
	c := &FoldersCore{}
	filter := clientDialogFilter(&mtproto.DialogFilter{
		PredicateName: mtproto.Predicate_dialogFilterChatlist,
		Id:            4,
		Title:         "Shared",
		HasMyInvites:  true,
		IncludePeers: []*mtproto.InputPeer{
			mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 8}).To_InputPeer(),
		},
	})
	inv := storedInvite{
		Slug:  "c7_4_0123456789abcdef",
		Title: "Shared link",
		Peers: []peerRef{{Kind: "channel", ID: 8}},
	}
	exported := mtproto.MakeTLChatlistsExportedChatlistInvite(&mtproto.Chatlists_ExportedChatlistInvite{
		Filter: filter,
		Invite: c.exportedInvite(filter, inv),
	}).To_Chatlists_ExportedChatlistInvite()
	mustEncode(t, exported)

	list := mtproto.MakeTLChatlistsExportedInvites(&mtproto.Chatlists_ExportedInvites{
		Invites: []*mtproto.ExportedChatlistInvite{c.exportedInvite(filter, inv)},
		Chats:   []*mtproto.Chat{},
		Users:   []*mtproto.User{},
	}).To_Chatlists_ExportedInvites()
	mustEncode(t, list)

	card := c.inviteCard(filter, inv, nil)
	mustEncode(t, card)
	if card.GetPredicateName() != mtproto.Predicate_chatlists_chatlistInvite {
		t.Fatalf("card %s", card.GetPredicateName())
	}
	already := c.inviteCard(filter, inv, filter)
	mustEncode(t, already)
	if already.GetPredicateName() != mtproto.Predicate_chatlists_chatlistInviteAlready {
		t.Fatalf("already %s", already.GetPredicateName())
	}

	updates := emptyChatlistUpdates()
	mustEncode(t, updates)
	peers := &mtproto.Vector_Peer{Datas: peersOf(inv.Peers)}
	mustEncode(t, peers)
	archived := folderPeerUpdates([]*mtproto.FolderPeer{
		mtproto.MakeTLFolderPeer(&mtproto.FolderPeer{
			Peer:     peerRef{Kind: "channel", ID: 8}.peer(),
			FolderId: 1,
		}).To_FolderPeer(),
	})
	mustEncode(t, archived)
	if archived.GetUpdates()[0].GetPtsCount() != 0 {
		t.Fatal("pts_count")
	}
	mustEncode(t, mtproto.MakeEmptyUpdates())
	mustEncode(t, updatesOf((&FoldersCore{}).filterUpdate(4, nil)))
}

func TestChatlistValidation(t *testing.T) {
	c := &FoldersCore{}
	if _, err := c.ChatlistsExportChatlistInvite(&mtproto.TLChatlistsExportChatlistInvite{}); err != mtproto.ErrFilterIdInvalid {
		t.Fatalf("export id: %v", err)
	}
	if _, err := c.ChatlistsExportChatlistInvite(&mtproto.TLChatlistsExportChatlistInvite{
		Chatlist: mtproto.MakeTLInputChatlistDialogFilter(&mtproto.InputChatlist{FilterId: 2}).To_InputChatlist(),
	}); err != mtproto.ErrPeersListEmpty {
		t.Fatalf("export peers: %v", err)
	}
	long := strings.Repeat("n", chatlistInviteTitleMax+1)
	if _, err := c.ChatlistsExportChatlistInvite(&mtproto.TLChatlistsExportChatlistInvite{
		Chatlist: mtproto.MakeTLInputChatlistDialogFilter(&mtproto.InputChatlist{FilterId: 2}).To_InputChatlist(),
		Title:    long,
		Peers:    []*mtproto.InputPeer{mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 1}).To_InputPeer()},
	}); err != mtproto.ErrMessageTooLong {
		t.Fatalf("title: %v", err)
	}
	if _, err := c.ChatlistsCheckChatlistInvite(&mtproto.TLChatlistsCheckChatlistInvite{}); err != mtproto.ErrInviteSlugEmpty {
		t.Fatalf("check: %v", err)
	}
	if _, err := c.ChatlistsCheckChatlistInvite(&mtproto.TLChatlistsCheckChatlistInvite{Slug: "nope"}); err != mtproto.ErrInviteSlugExpired {
		t.Fatalf("expired: %v", err)
	}
	if _, err := c.FoldersEditPeerFolders(&mtproto.TLFoldersEditPeerFolders{}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.FoldersEditPeerFolders(&mtproto.TLFoldersEditPeerFolders{
		FolderPeers: []*mtproto.InputFolderPeer{
			mtproto.MakeTLInputFolderPeer(&mtproto.InputFolderPeer{
				Peer:     mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 3}).To_InputPeer(),
				FolderId: 2,
			}).To_InputFolderPeer(),
		},
	}); err != mtproto.ErrFolderIdInvalid {
		t.Fatalf("folder: %v", err)
	}
	if _, err := c.ChatlistsDeleteExportedInvite(&mtproto.TLChatlistsDeleteExportedInvite{
		Chatlist: mtproto.MakeTLInputChatlistDialogFilter(&mtproto.InputChatlist{FilterId: 0}).To_InputChatlist(),
	}); err != mtproto.ErrFilterIdInvalid {
		t.Fatalf("delete: %v", err)
	}
	if _, err := c.ChatlistsJoinChatlistInvite(&mtproto.TLChatlistsJoinChatlistInvite{}); err != mtproto.ErrInviteSlugEmpty {
		t.Fatalf("join: %v", err)
	}
	if _, err := chatlistFilterID(nil); err != mtproto.ErrFilterIdInvalid {
		t.Fatal(err)
	}
	if _, err := refFromInput(1, mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 0}).To_InputPeer()); err != mtproto.ErrPeerIdInvalid {
		t.Fatal("user")
	}
	ref, err := refFromInput(1, mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 9}).To_InputPeer())
	if err != nil || ref.Kind != "channel" || ref.dialogID() >= 0 {
		t.Fatalf("channel ref %+v %v", ref, err)
	}
}

func TestMissingAndHidePeers(t *testing.T) {
	inv := storedInvite{Peers: []peerRef{{Kind: "channel", ID: 1}, {Kind: "chat", ID: 2}}}
	joined := &mtproto.DialogFilter{
		IncludePeers: []*mtproto.InputPeer{
			mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 1}).To_InputPeer(),
		},
	}
	missing := missingRefs(inv, joined, nil)
	if len(missing) != 1 || missing[0].ID != 2 {
		t.Fatalf("missing %+v", missing)
	}
	hidden := missingRefs(inv, joined, []peerRef{{Kind: "chat", ID: 2}})
	if len(hidden) != 0 {
		t.Fatalf("hidden %+v", hidden)
	}
	left := subtractRefs(inv.Peers, []peerRef{{Kind: "channel", ID: 1}})
	if len(left) != 1 || left[0].ID != 2 {
		t.Fatalf("subtract %+v", left)
	}
}

type encoder interface {
	Encode(x *mtproto.EncodeBuf, layer int32) error
}

func mustEncode(t *testing.T, v encoder) {
	t.Helper()
	buf := mtproto.NewEncodeBuf(512)
	if err := v.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
	if len(buf.GetBuf()) < 4 {
		t.Fatalf("short %d", len(buf.GetBuf()))
	}
}
