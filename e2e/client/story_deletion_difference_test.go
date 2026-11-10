package e2e

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestOfflineDialogPeerLearnsStoryDeletion is the missed live-story ring.
// B is not A's contact. They only share a private chat and B has opened A's
// profile. B goes offline, the live ends, and B's updates.getDifference on
// the same session carries storyItemDeleted plus a user with
// stories_unavailable. stories.getAllStories must not stay NotModified on
// the empty-tray hash from before the deletion.
func TestOfflineDialogPeerLearnsStoryDeletion(t *testing.T) {
	cfg := requireE2E(t)
	ensureStoryAdvert(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()

	host, hostSelf := dialPhone(t, ctx, cfg, "9996610096", "StoryHost")
	viewer, err := Dial(ctx, cfg, ProfileIOS)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = viewer.Login(ctx, Phone{Number: "9996610097", Code: "11111"}, "StoryViewer"); err != nil {
		t.Fatal(err)
	}
	name := claimUsername(t, ctx, host)
	peer, contact := resolveUser(t, ctx, viewer, name)
	if peer.GetUserId() != hostSelf.ID {
		t.Fatalf("resolved %d, host %d", peer.GetUserId(), hostSelf.ID)
	}
	if contact {
		t.Fatal("viewer became a contact; getAllStories would include the host")
	}
	if _, err = viewer.SendText(ctx, peer, "private chat", nil); err != nil {
		t.Fatal(err)
	}
	if err = waitUntil(ctx, 200*time.Millisecond, func() (bool, error) {
		dialogs, err := viewer.Dialogs(ctx)
		if err != nil {
			return false, err
		}
		for _, d := range dialogs {
			if d.GetPeer().GetUserId() == hostSelf.ID {
				return true, nil
			}
		}
		return false, nil
	}); err != nil {
		t.Fatal(err)
	}

	before, beforeNotModified, ok := storiesState(t, ctx, viewer, "")
	if !ok || beforeNotModified || before == "" {
		t.Fatalf("initial stories state modified=%v ok=%v state=%q", beforeNotModified, ok, before)
	}

	storyID, callID, access := startPublicLive(t, ctx, host)
	if err = waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return peerHasLiveStory(ctx, viewer, peer, storyID), nil
	}); err != nil {
		t.Fatalf("profile never showed live story %d: %v", storyID, err)
	}
	pts, qts, date, err := viewer.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	sess, ok := viewer.Session()
	if !ok {
		t.Fatal("viewer session was not issued")
	}
	viewer.Close()

	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(1100 * time.Millisecond):
	}
	if _, err = host.Invoke(ctx, &mtproto.TLPhoneDiscardGroupCall{
		Constructor: mtproto.CRC32_phone_discardGroupCall,
		Call:        mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: callID, AccessHash: access}).To_InputGroupCall(),
	}); err != nil {
		t.Fatal(err)
	}

	back, err := DialSession(ctx, cfg, ProfileIOS, sess)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(back.Close)
	diff, err := back.Difference(ctx, pts, qts, date)
	if err != nil {
		t.Fatalf("updates.getDifference: %v", err)
	}
	ups, users, state := differenceCatchUp(t, diff)
	if !storyDeletedUpdate(ups, hostSelf.ID, storyID) {
		t.Fatalf("getDifference missed story %d: %T updates=%d", storyID, diff, len(ups))
	}
	owner := userByID(users, hostSelf.ID)
	if owner == nil || ownerStillAdvertises(owner, storyID) {
		t.Fatalf("owner user %+v", owner)
	}
	if state.GetPts() != 0 && pts != 0 && state.GetPts() < pts {
		t.Fatalf("difference rewound pts from %d to %d", pts, state.GetPts())
	}

	after, afterNotModified, ok := storiesState(t, ctx, back, before)
	if !ok || afterNotModified {
		t.Fatalf("getAllStories stayed NotModified at %q", before)
	}
	if after == before {
		t.Fatalf("tray hash did not change from %q", before)
	}
}

func claimUsername(t *testing.T, ctx context.Context, c *Client) string {
	t.Helper()
	var last error
	for i := 0; i < 8; i++ {
		name := "lv" + randHex(6)
		updated, err := c.Invoke(ctx, &mtproto.TLAccountUpdateUsername{
			Constructor: mtproto.CRC32_account_updateUsername,
			Username:    name,
		})
		if err != nil {
			if rpc, ok := asRPCError(err); ok && rpc.Message == "USERNAME_OCCUPIED" {
				last = err
				continue
			}
			t.Fatal(err)
		}
		if _, ok := updated.(*mtproto.TLUser); !ok {
			t.Fatalf("account.updateUsername returned %T", updated)
		}
		return name
	}
	t.Fatalf("could not claim a username: %v", last)
	return ""
}

func resolveUser(t *testing.T, ctx context.Context, c *Client, username string) (*mtproto.InputPeer, bool) {
	t.Helper()
	res, err := c.Invoke(ctx, &mtproto.TLContactsResolveUsername{
		Constructor: mtproto.CRC32_contacts_resolveUsername_725afbbc,
		Username:    username,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolved, ok := res.(*mtproto.TLContactsResolvedPeer)
	if !ok || resolved.GetPeer().GetUserId() == 0 {
		t.Fatalf("contacts.resolveUsername returned %T", res)
	}
	id := resolved.GetPeer().GetUserId()
	var hash int64
	contact := false
	for _, user := range resolved.GetUsers() {
		if user == nil || user.GetId() != id {
			continue
		}
		contact = user.GetContact()
		if user.GetAccessHash() != nil {
			hash = user.GetAccessHash().GetValue()
		}
	}
	if hash == 0 {
		t.Fatal("resolved user has no access hash")
	}
	return inputPeerUser(id, hash), contact
}

func startPublicLive(t *testing.T, ctx context.Context, host *Client) (storyID int32, callID, access int64) {
	t.Helper()
	res := invokeOK(t, host, &mtproto.TLStoriesStartLive{
		Constructor: mtproto.CRC32_stories_startLive,
		Peer:        inputPeerSelf(),
		RandomId:    time.Now().UnixNano(),
		PrivacyRules: []*mtproto.InputPrivacyRule{
			mtproto.MakeTLInputPrivacyValueAllowAll(nil).To_InputPrivacyRule(),
		},
	})
	for _, u := range updatesOf(res) {
		if u.GetStory().GetId() != 0 && u.GetStory().GetPredicateName() != mtproto.Predicate_storyItemDeleted {
			storyID = u.GetStory().GetId()
		}
		call := u.GetCall_GROUPCALL()
		if call != nil && call.GetId() != 0 {
			callID = call.GetId()
			access = call.GetAccessHash()
		}
	}
	if storyID == 0 || callID == 0 {
		t.Fatalf("startLive story=%d call=%d response %T", storyID, callID, res)
	}
	return storyID, callID, access
}

func storiesState(t *testing.T, ctx context.Context, c *Client, state string) (got string, notModified, ok bool) {
	t.Helper()
	res, err := c.Invoke(ctx, &mtproto.TLStoriesGetAllStories{
		Constructor: mtproto.CRC32_stories_getAllStories,
		State:       &wrapperspb.StringValue{Value: state},
	})
	if err != nil {
		t.Fatal(err)
	}
	switch v := res.(type) {
	case *mtproto.TLStoriesAllStoriesNotModified:
		return v.GetState(), true, true
	case *mtproto.TLStoriesAllStories:
		return v.GetState(), false, true
	case *mtproto.Stories_AllStories:
		return v.GetState(), v.GetPredicateName() == mtproto.Predicate_stories_allStoriesNotModified, true
	default:
		return "", false, false
	}
}

func differenceCatchUp(t *testing.T, obj mtproto.TLObject) ([]*mtproto.Update, []*mtproto.User, *mtproto.Updates_State) {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLUpdatesDifference:
		return v.GetOtherUpdates(), v.GetUsers(), v.GetState()
	case *mtproto.TLUpdatesDifferenceSlice:
		return v.GetOtherUpdates(), v.GetUsers(), v.GetIntermediateState()
	case *mtproto.Updates_Difference:
		state := v.GetState()
		if state == nil {
			state = v.GetIntermediateState()
		}
		return v.GetOtherUpdates(), v.GetUsers(), state
	default:
		t.Fatalf("updates.getDifference returned %T", obj)
		return nil, nil, nil
	}
}

func peerHasLiveStory(ctx context.Context, viewer *Client, peer *mtproto.InputPeer, storyID int32) bool {
	res, err := viewer.Invoke(ctx, &mtproto.TLStoriesGetPeerStories{
		Constructor: mtproto.CRC32_stories_getPeerStories,
		Peer:        peer,
	})
	if err != nil {
		return false
	}
	for _, item := range peerStoryItems(res) {
		if item.GetId() == storyID && item.GetPredicateName() != mtproto.Predicate_storyItemDeleted {
			return true
		}
	}
	full, err := viewer.Invoke(ctx, &mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(peer.GetUserId(), peer.GetAccessHash()),
	})
	if err != nil {
		return false
	}
	user := userByID(usersOf(full), peer.GetUserId())
	recent := user.GetStoriesMaxId_FLAGRECENTSTORY()
	return recent.GetLive() && recent.GetMaxId().GetValue() == storyID
}

func ownerStillAdvertises(user *mtproto.User, storyID int32) bool {
	if user == nil {
		return true
	}
	recent := user.GetStoriesMaxId_FLAGRECENTSTORY()
	if recent.GetLive() || recent.GetMaxId().GetValue() == storyID {
		return true
	}
	// No story left on this user. The difference has to say so, or the
	// client keeps the cached ring.
	if recent == nil || recent.GetMaxId().GetValue() == 0 {
		return !user.GetStoriesUnavailable()
	}
	return false
}

func storyDeletedUpdate(ups []*mtproto.Update, owner int64, id int32) bool {
	for _, u := range ups {
		item := u.GetStory()
		if u.GetPredicateName() == mtproto.Predicate_updateStory &&
			u.GetPeer_PEER().GetUserId() == owner &&
			item.GetPredicateName() == mtproto.Predicate_storyItemDeleted &&
			item.GetId() == id {
			return true
		}
	}
	return false
}

func randHex(n int) string {
	const hexdigits = "0123456789abcdef"
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "abcdef"
	}
	out := make([]byte, n)
	for i := range buf {
		out[i] = hexdigits[int(buf[i])%len(hexdigits)]
	}
	return string(out)
}
