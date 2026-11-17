package e2e

import (
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestLinkedUserSeesTheCommunity is communities.create with a user peer.
// The other user must see that community in getJoinedCommunities.
func TestLinkedUserSeesTheCommunity(t *testing.T) {
	cfg := requireE2E(t)
	a, _ := loginNamed(t, cfg, ProfileIOS, "comm_a", "CommA")
	b, bSelf := loginNamed(t, cfg, ProfileAndroid, "comm_b", "CommB")
	ctx, cancel := contextTimeout(2 * time.Minute)
	defer cancel()

	created, err := a.Invoke(ctx, &mtproto.TLCommunitiesCreate{
		Constructor: mtproto.CRC32_communities_create,
		Title:       "comm-see",
		Peer:        inputPeerUser(bSelf.ID, bSelf.AccessHash),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !objectHasChatTitle(created, "comm-see") {
		t.Fatalf("create result has no community: %T", created)
	}
	joined, err := b.Invoke(ctx, &mtproto.TLCommunitiesGetJoinedCommunities{
		Constructor: mtproto.CRC32_communities_getJoinedCommunities,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !objectHasChatTitle(joined, "comm-see") {
		t.Fatalf("linked user cannot see the community: %T", joined)
	}
}

func objectHasChatTitle(obj mtproto.TLObject, title string) bool {
	switch v := obj.(type) {
	case *mtproto.TLUpdates:
		obj = v.To_Updates()
	case *mtproto.TLMessagesChats:
		obj = v.To_Messages_Chats()
	}
	switch v := obj.(type) {
	case *mtproto.Updates:
		for _, ch := range v.GetChats() {
			if ch.GetTitle() == title {
				return true
			}
		}
	case *mtproto.Messages_Chats:
		for _, ch := range v.GetChats() {
			if ch.GetTitle() == title {
				return true
			}
		}
	}
	return false
}
