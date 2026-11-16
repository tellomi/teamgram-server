package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// Phones 9996610463 and 9996610464 belong to this task's range.

func TestContactsSearchNameAndNotes(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	searcher, self := dialPhone(t, ctx, cfg, "9996610463", "Searcher")
	target, targetSelf := dialPhone(t, ctx, cfg, "9996610464", "Ada")
	if _, err := target.Invoke(ctx, &mtproto.TLAccountUpdateProfile{
		Constructor: mtproto.CRC32_account_updateProfile,
		FirstName:   strFlag("Ada"),
		LastName:    strFlag("Lovelace"),
	}); err != nil {
		t.Fatal(err)
	}

	miss := searchContacts(t, ctx, searcher, "zzzqqq")
	if peerListHas(miss.GetMyResults(), self.ID) || peerListHas(miss.GetResults(), self.ID) {
		t.Fatalf("caller is in an unrelated search: %#v %#v", miss.GetMyResults(), miss.GetResults())
	}

	byName := searchContacts(t, ctx, searcher, "Ada Lovelace")
	if !peerListHas(byName.GetResults(), targetSelf.ID) && !peerListHas(byName.GetMyResults(), targetSelf.ID) {
		t.Fatalf("full name missing: results=%v my=%v", peerIDs(byName.GetResults()), peerIDs(byName.GetMyResults()))
	}
	if peerListHas(byName.GetResults(), self.ID) || peerListHas(byName.GetMyResults(), self.ID) {
		t.Fatal("caller is in the full-name search")
	}

	if _, err := searcher.Invoke(ctx, &mtproto.TLContactsImportContacts{
		Constructor: mtproto.CRC32_contacts_importContacts,
		Contacts: []*mtproto.InputContact{
			mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{
				ClientId:  1,
				Phone:     "9996610464",
				FirstName: "Notebook",
				LastName:  "Pal",
			}).To_InputContact(),
		},
	}); err != nil {
		t.Fatal(err)
	}
	byNote := searchContacts(t, ctx, searcher, "Notebook Pal")
	if !peerListHas(byNote.GetMyResults(), targetSelf.ID) {
		t.Fatalf("note is not in my_results: my=%v results=%v", peerIDs(byNote.GetMyResults()), peerIDs(byNote.GetResults()))
	}
}

func searchContacts(t *testing.T, ctx context.Context, c *Client, q string) *mtproto.Contacts_Found {
	t.Helper()
	return foundOf(t, invokeOK(t, c, &mtproto.TLContactsSearch{
		Constructor: mtproto.CRC32_contacts_search_5f58d0f,
		Q:           q,
		Limit:       20,
	}))
}

func peerListHas(peers []*mtproto.Peer, id int64) bool {
	for _, peer := range peers {
		if peer.GetUserId() == id {
			return true
		}
	}
	return false
}

func peerIDs(peers []*mtproto.Peer) []int64 {
	out := make([]int64, 0, len(peers))
	for _, peer := range peers {
		out = append(out, peer.GetUserId())
	}
	return out
}
