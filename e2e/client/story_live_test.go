package e2e

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestLiveStubClearsWhenTheStreamEnds is the chat-list ring the owner reported.
// stories.startLive stores an empty live story. While the call is up, other
// accounts see recentStory.live. Discard, a host that never connects, and a
// restart that drops the in-memory call all have to delete that stub and push
// an update so the ring is gone. A stub must not stay live with nothing to open.
func TestLiveStubClearsWhenTheStreamEnds(t *testing.T) {
	a, b, aSelf, bSelf := sharedPair(t)
	ensureStoryAdvert(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	// A login that has not called updates.getState is not an updates session.
	// The server has nowhere to deliver storyItemDeleted until this returns.
	if _, _, _, err := b.State(ctx); err != nil {
		t.Fatal(err)
	}
	peerForB := inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)

	storyID, callID, access := startLive(t, ctx, a, bSelf)
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return viewerSeesStory(ctx, b, aSelf.ID, peerForB, storyID), nil
	}); err != nil {
		t.Fatalf("contact never saw live story %d: %v", storyID, err)
	}

	if _, err := a.Invoke(ctx, &mtproto.TLPhoneDiscardGroupCall{
		Constructor: mtproto.CRC32_phone_discardGroupCall,
		Call:        mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: callID, AccessHash: access}).To_InputGroupCall(),
	}); err != nil {
		t.Fatal(err)
	}
	clearCtx, clearCancel := contextTimeout(20 * time.Second)
	defer clearCancel()
	if err := waitUntil(clearCtx, 300*time.Millisecond, func() (bool, error) {
		if viewerSeesStory(clearCtx, b, aSelf.ID, peerForB, storyID) {
			return false, nil
		}
		return pushedStoryCleared(b.Pushed(), storyID, aSelf.ID), nil
	}); err != nil {
		t.Fatalf("discard story %d: stillUp=%v pushes=%s", storyID, viewerSeesStory(clearCtx, b, aSelf.ID, peerForB, storyID), pushSummary(b.Pushed()))
	}

	storyID, _, _ = startLive(t, ctx, a, bSelf)
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return viewerSeesStory(ctx, b, aSelf.ID, peerForB, storyID), nil
	}); err != nil {
		t.Fatalf("second live story %d was not visible: %v", storyID, err)
	}
	// The host never joins. The server ends the room on its own.
	timeoutCtx, timeoutCancel := contextTimeout(45 * time.Second)
	defer timeoutCancel()
	if err := waitUntil(timeoutCtx, time.Second, func() (bool, error) {
		return !viewerSeesStory(timeoutCtx, b, aSelf.ID, peerForB, storyID), nil
	}); err != nil {
		t.Fatalf("host join timeout left story %d up: %v", storyID, err)
	}
}

// TestOrphanLiveStubRepair is a stream whose group call is gone after a
// process restart, which is the shape of the leftover live stub in production.
func TestOrphanLiveStubRepair(t *testing.T) {
	if os.Getenv("TELLOMI_E2E_RESTART") == "" {
		t.Skip("set TELLOMI_E2E_RESTART=1 to restart the stack")
	}
	a, b, aSelf, _ := sharedPair(t)
	ensureStoryAdvert(t)
	ctx, cancel := contextTimeout(3 * time.Minute)
	defer cancel()
	peerForB := inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)
	storyID, _, _ := startLive(t, ctx, a, bobSelf)
	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		return viewerSeesStory(ctx, b, aSelf.ID, peerForB, storyID), nil
	}); err != nil {
		t.Fatalf("live story %d before restart: %v", storyID, err)
	}
	if err := restartTeamgram(ctx); err != nil {
		t.Fatal(err)
	}
	addr := os.Getenv("TELLOMI_E2E_ADDR")
	if addr == "" {
		addr = "127.0.0.1:10443"
	}
	// The published port stays open while the process is stopping. Wait
	// until it drops, then until the new process is actually accepting RPCs,
	// or every later test dials a half-started gateway.
	_ = waitTCPDown(ctx, addr)
	if err := waitTCP(ctx, addr); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	case <-time.After(8 * time.Second):
	}
	a, b, aSelf, _ = sharedPair(t)
	peerForB = inputPeerUser(aliceSeen.ID, aliceSeen.AccessHash)
	ctx, cancel = contextTimeout(2 * time.Minute)
	defer cancel()
	if err := waitUntil(ctx, time.Second, func() (bool, error) {
		return !viewerSeesStory(ctx, b, aSelf.ID, peerForB, storyID), nil
	}); err != nil {
		t.Fatalf("orphaned live story %d still on the ring after restart: %v", storyID, err)
	}
}

func startLive(t *testing.T, ctx context.Context, host *Client, viewer Self) (storyID int32, callID, access int64) {
	t.Helper()
	res := invokeOK(t, host, &mtproto.TLStoriesStartLive{
		Constructor: mtproto.CRC32_stories_startLive,
		Peer:        inputPeerSelf(),
		RandomId:    time.Now().UnixNano(),
		PrivacyRules: []*mtproto.InputPrivacyRule{
			mtproto.MakeTLInputPrivacyValueAllowAll(nil).To_InputPrivacyRule(),
			mtproto.MakeTLInputPrivacyValueAllowUsers(&mtproto.InputPrivacyRule{
				Users: []*mtproto.InputUser{inputUser(viewer.ID, viewer.AccessHash)},
			}).To_InputPrivacyRule(),
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

func updatesOf(obj mtproto.TLObject) []*mtproto.Update {
	switch v := obj.(type) {
	case *mtproto.TLUpdates:
		return v.GetUpdates()
	case *mtproto.TLUpdatesCombined:
		return v.GetUpdates()
	case *mtproto.Updates:
		return v.GetUpdates()
	default:
		return nil
	}
}

func viewerSeesStory(ctx context.Context, viewer *Client, host int64, peer *mtproto.InputPeer, storyID int32) bool {
	if storyID != 0 {
		res, err := viewer.Invoke(ctx, &mtproto.TLStoriesGetPeerStories{
			Constructor: mtproto.CRC32_stories_getPeerStories,
			Peer:        peer,
		})
		if err == nil {
			for _, item := range peerStoryItems(res) {
				if item.GetId() == storyID && item.GetPredicateName() != mtproto.Predicate_storyItemDeleted {
					return true
				}
			}
		}
	}
	return userHasRing(ctx, viewer, host, peer)
}

func userHasRing(ctx context.Context, viewer *Client, host int64, peer *mtproto.InputPeer) bool {
	ask := func(req mtproto.TLObject) bool {
		res, err := viewer.Invoke(ctx, req)
		if err != nil {
			return false
		}
		return ringLive(userByID(usersOf(res), host))
	}
	if ask(&mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUser(peer.GetUserId(), peer.GetAccessHash())},
	}) {
		return true
	}
	if ask(&mtproto.TLUsersGetFullUser{
		Constructor: mtproto.CRC32_users_getFullUser,
		Id:          inputUser(peer.GetUserId(), peer.GetAccessHash()),
	}) {
		return true
	}
	if ask(&mtproto.TLMessagesGetDialogs{
		Constructor: mtproto.CRC32_messages_getDialogs,
		OffsetPeer:  inputPeerEmpty(),
		Limit:       100,
	}) {
		return true
	}
	res, err := viewer.Invoke(ctx, &mtproto.TLStoriesGetPeerMaxIDs78499170{
		Constructor: mtproto.CRC32_stories_getPeerMaxIDs78499170,
		Id:          []*mtproto.InputPeer{peer},
	})
	if err == nil {
		switch v := res.(type) {
		case *tlVector:
			for _, story := range v.Stories {
				if story.GetLive() || story.GetMaxId().GetValue() != 0 {
					return true
				}
			}
		case *mtproto.Vector_RecentStory:
			if len(v.GetDatas()) > 0 && (v.Datas[0].GetLive() || v.Datas[0].GetMaxId().GetValue() != 0) {
				return true
			}
		}
	}
	peerStories, err := viewer.Invoke(ctx, &mtproto.TLStoriesGetPeerStories{
		Constructor: mtproto.CRC32_stories_getPeerStories,
		Peer:        peer,
	})
	if err != nil {
		return false
	}
	var stories []*mtproto.StoryItem
	switch v := peerStories.(type) {
	case *mtproto.TLStoriesPeerStories:
		stories = v.GetStories().GetStories()
	case *mtproto.Stories_PeerStories:
		stories = v.GetStories().GetStories()
	}
	for _, item := range stories {
		if item.GetPredicateName() == mtproto.Predicate_storyItemDeleted {
			continue
		}
		if item.GetId() != 0 {
			return true
		}
	}
	return false
}

func peerStoryItems(obj mtproto.TLObject) []*mtproto.StoryItem {
	switch v := obj.(type) {
	case *mtproto.TLStoriesPeerStories:
		return v.GetStories().GetStories()
	case *mtproto.Stories_PeerStories:
		return v.GetStories().GetStories()
	default:
		return nil
	}
}

func pushedStoryCleared(objs []mtproto.TLObject, storyID int32, host int64) bool {
	for _, obj := range objs {
		for _, u := range updatesOf(obj) {
			item := u.GetStory()
			if item.GetId() == storyID && item.GetPredicateName() == mtproto.Predicate_storyItemDeleted {
				return true
			}
			if u.GetPredicateName() == mtproto.Predicate_updateUser && u.GetUserId() == host {
				if user := userByID(usersOf(obj), host); user != nil && !ringLive(user) {
					return true
				}
			}
			if u.GetLiveStory() && u.GetCall_GROUPCALL().GetPredicateName() == mtproto.Predicate_groupCallDiscarded {
				return true
			}
		}
	}
	return false
}

func pushSummary(objs []mtproto.TLObject) string {
	n := len(objs)
	if n > 8 {
		objs = objs[n-8:]
	}
	parts := make([]string, 0, len(objs))
	for _, obj := range objs {
		parts = append(parts, obj.String())
	}
	return strings.Join(parts, " | ")
}

func ringLive(user *mtproto.User) bool {
	if user == nil {
		return false
	}
	recent := user.GetStoriesMaxId_FLAGRECENTSTORY()
	if recent == nil {
		return false
	}
	return recent.GetLive() || recent.GetMaxId().GetValue() != 0
}

func usersOf(obj mtproto.TLObject) []*mtproto.User {
	switch v := obj.(type) {
	case *mtproto.TLUpdates:
		return v.GetUsers()
	case *mtproto.TLUpdatesCombined:
		return v.GetUsers()
	case *mtproto.Updates:
		return v.GetUsers()
	case *mtproto.Vector_User:
		return v.GetDatas()
	case *tlVector:
		return v.Users
	case *mtproto.TLUsersUserFull:
		return v.GetUsers()
	case *mtproto.Users_UserFull:
		return v.GetUsers()
	case *mtproto.TLMessagesDialogs:
		return v.GetUsers()
	case *mtproto.TLMessagesDialogsSlice:
		return v.GetUsers()
	case *mtproto.Messages_Dialogs:
		return v.GetUsers()
	default:
		return nil
	}
}

func userByID(users []*mtproto.User, id int64) *mtproto.User {
	for _, u := range users {
		if u.GetId() == id {
			return u
		}
	}
	return nil
}

func ensureStoryAdvert(t *testing.T) {
	t.Helper()
	if os.Getenv("TELLOMI_E2E_RESTART") == "" && os.Getenv("TELLOMI_E2E_COMPOSE_FILES") == "" {
		return
	}
	sql := `
CREATE TABLE IF NOT EXISTS story_advert_dirty (
  user_id BIGINT NOT NULL,
  stories_max_id INT NOT NULL,
  updated_at INT NOT NULL,
  PRIMARY KEY (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
CREATE TABLE IF NOT EXISTS story_live_owner (
  owner_id BIGINT NOT NULL,
  live_max INT NOT NULL,
  PRIMARY KEY (owner_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
`
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	args := composeArgs("exec", "-T", "mysql", "mysql", "-uroot", "-proot", "teamgram")
	cmd := exec.CommandContext(ctx, "docker", args...)
	if dir := os.Getenv("TELLOMI_E2E_ROOT"); dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = strings.NewReader(sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("story advert tables: %v\n%s", err, out)
	}
}
