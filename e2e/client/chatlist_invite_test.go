package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestChatlistInviteJoinsChannel shares a normal dialogFilter that holds a
// megagroup. Export converts it to dialogFilterChatlist. checkChatlistInvite
// returns the channel with access_hash. joinChatlistInvite adds the joiner.
// editExportedInvite accepts a second megagroup that was not in include_peers.
func TestChatlistInviteJoinsChannel(t *testing.T) {
	cfg := requireE2E(t)
	owner, _ := loginNamed(t, cfg, ProfileAndroid, "chatlist_owner", "ListOwner")
	joiner, _ := loginNamed(t, cfg, ProfileIOS, "chatlist_joiner", "ListJoiner")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	t.Cleanup(func() { clearCustomFilters(t, owner) })

	firstID, firstHash, err := owner.CreateMegagroup(ctx, "e2e-folder-room")
	if err != nil {
		t.Fatal(err)
	}
	secondID, secondHash, err := owner.CreateMegagroup(ctx, "e2e-folder-extra")
	if err != nil {
		t.Fatal(err)
	}
	invokeOK(t, owner, &mtproto.TLMessagesUpdateDialogFilter{
		Constructor: mtproto.CRC32_messages_updateDialogFilter,
		Id:          2,
		Filter: mtproto.MakeTLDialogFilter(&mtproto.DialogFilter{
			Id:       2,
			Contacts: true,
			Title_TEXTWITHENTITIES: mtproto.MakeTLTextWithEntities(&mtproto.TextWithEntities{
				Text:     "Rooms",
				Entities: []*mtproto.MessageEntity{},
			}).To_TextWithEntities(),
			IncludePeers: []*mtproto.InputPeer{inputPeerChannel(firstID, firstHash)},
		}).To_DialogFilter(),
	})

	exported := invokeOK(t, owner, &mtproto.TLChatlistsExportChatlistInvite{
		Constructor: mtproto.CRC32_chatlists_exportChatlistInvite,
		Chatlist:    mtproto.MakeTLInputChatlistDialogFilter(&mtproto.InputChatlist{FilterId: 2}).To_InputChatlist(),
		Title:       "Rooms link",
		Peers:       []*mtproto.InputPeer{inputPeerChannel(firstID, firstHash)},
	})
	link := exportedInviteOf(t, exported)
	if link.GetFilter().GetPredicateName() != mtproto.Predicate_dialogFilterChatlist {
		t.Fatalf("exported filter %s", link.GetFilter().GetPredicateName())
	}
	slug := slugOf(t, link.GetInvite().GetUrl())

	checked := invokeOK(t, joiner, &mtproto.TLChatlistsCheckChatlistInvite{
		Constructor: mtproto.CRC32_chatlists_checkChatlistInvite,
		Slug:        slug,
	})
	invite := chatlistInviteOf(t, checked)
	if len(invite.GetPeers()) == 0 {
		t.Fatal("check peers empty")
	}
	var joinPeer *mtproto.InputPeer
	for _, chat := range invite.GetChats() {
		if chat.GetId() != firstID {
			continue
		}
		hash := chatAccessHash(chat)
		if hash == 0 {
			t.Fatalf("channel card without access_hash %+v", chat)
		}
		joinPeer = inputPeerChannel(chat.GetId(), hash)
	}
	if joinPeer == nil {
		t.Fatalf("check chats %+v", invite.GetChats())
	}
	invokeOK(t, joiner, &mtproto.TLChatlistsJoinChatlistInvite{
		Constructor: mtproto.CRC32_chatlists_joinChatlistInvite,
		Slug:        slug,
		Peers:       []*mtproto.InputPeer{joinPeer},
	})
	part, err := joiner.Invoke(ctx, &mtproto.TLChannelsGetParticipant{
		Constructor: mtproto.CRC32_channels_getParticipant,
		Channel:     inputChannel(firstID, joinPeer.GetAccessHash()),
		Participant: inputPeerSelf(),
	})
	if err != nil {
		t.Fatalf("joiner is not in the megagroup: %v", err)
	}
	if part == nil {
		t.Fatal("empty participant")
	}

	edited := invokeOK(t, owner, &mtproto.TLChatlistsEditExportedInvite{
		Constructor: mtproto.CRC32_chatlists_editExportedInvite,
		Chatlist:    mtproto.MakeTLInputChatlistDialogFilter(&mtproto.InputChatlist{FilterId: 2}).To_InputChatlist(),
		Slug:        slug,
		Peers: []*mtproto.InputPeer{
			inputPeerChannel(firstID, firstHash),
			inputPeerChannel(secondID, secondHash),
		},
	})
	editedInvite, ok := edited.(*mtproto.TLExportedChatlistInvite)
	if !ok {
		if wrapped, wrapOK := edited.(*mtproto.ExportedChatlistInvite); wrapOK {
			editedInvite = mtproto.MakeTLExportedChatlistInvite(wrapped)
			ok = true
		}
	}
	if !ok {
		t.Fatalf("edit %T", edited)
	}
	var sawSecond bool
	for _, peer := range editedInvite.GetPeers() {
		if peer.GetChannelId() == secondID {
			sawSecond = true
		}
	}
	if !sawSecond {
		t.Fatalf("edited peers %+v", editedInvite.GetPeers())
	}
}

func exportedInviteOf(t *testing.T, obj mtproto.TLObject) *mtproto.TLChatlistsExportedChatlistInvite {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLChatlistsExportedChatlistInvite:
		return v
	case *mtproto.Chatlists_ExportedChatlistInvite:
		return mtproto.MakeTLChatlistsExportedChatlistInvite(v)
	default:
		t.Fatalf("export %T", obj)
	}
	return nil
}

func chatlistInviteOf(t *testing.T, obj mtproto.TLObject) *mtproto.TLChatlistsChatlistInvite {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLChatlistsChatlistInvite:
		return v
	case *mtproto.Chatlists_ChatlistInvite:
		if v.GetPredicateName() == mtproto.Predicate_chatlists_chatlistInviteAlready {
			t.Fatalf("already joined %+v", v)
		}
		return mtproto.MakeTLChatlistsChatlistInvite(v)
	default:
		t.Fatalf("check %T", obj)
	}
	return nil
}

func slugOf(t *testing.T, url string) string {
	t.Helper()
	const marker = "/addlist/"
	i := len(url) - 1
	for i >= 0 && url[i] != '/' {
		i--
	}
	if i < 0 || i+1 >= len(url) {
		t.Fatalf("invite url %q", url)
	}
	_ = marker
	return url[i+1:]
}
