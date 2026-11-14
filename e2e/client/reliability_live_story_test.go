//go:build reliability

package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

// TestReliabilityLiveStoryMedia is stories.startLive on the stack that has
// the story advertisement tables. Layer 232 viewers read
// story.media = messageMediaVideoStream with the group call. Ending the
// call still removes a stream that has no recording.
func TestReliabilityLiveStoryMedia(t *testing.T) {
	t.Parallel()
	s := useReliabilityStack(t)
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()

	hostPhone := reliabilityPhone("0361")
	viewPhone := reliabilityPhone("0362")
	host, _ := s.login(t, ProfileIOS, hostPhone, "LiveHost")
	viewer, viewerSelf := s.login(t, ProfileAndroid, viewPhone, "LiveView")
	viewID, viewHash, err := host.ImportPhone(ctx, viewPhone.Number, "LiveView")
	if err != nil {
		t.Fatal(err)
	}
	if viewID != viewerSelf.ID {
		t.Fatalf("imported viewer %d, login %d", viewID, viewerSelf.ID)
	}
	hostID, hostHash, err := viewer.ImportPhone(ctx, hostPhone.Number, "LiveHost")
	if err != nil {
		t.Fatal(err)
	}
	peer := inputPeerUser(hostID, hostHash)

	res := invokeOK(t, host, &mtproto.TLStoriesStartLive{
		Constructor: mtproto.CRC32_stories_startLive,
		RtmpStream:  true,
		Peer:        inputPeerSelf(),
		RandomId:    time.Now().UnixNano(),
		PrivacyRules: []*mtproto.InputPrivacyRule{
			mtproto.MakeTLInputPrivacyValueAllowAll(nil).To_InputPrivacyRule(),
			mtproto.MakeTLInputPrivacyValueAllowUsers(&mtproto.InputPrivacyRule{
				Users: []*mtproto.InputUser{inputUser(viewID, viewHash)},
			}).To_InputPrivacyRule(),
		},
	})
	storyID, callID, access := liveCallOf(t, res)
	// storyItem#16a4b93c has no live flag. The stream is the media.
	assertClientLiveStream(t, storyFromTL(res, storyID), callID, access, true)
	assertRecentLiveIfPresent(t, res, hostID)

	if err := waitUntil(ctx, 300*time.Millisecond, func() (bool, error) {
		item, ok := viewerStory(ctx, viewer, peer, storyID)
		if !ok {
			return false, nil
		}
		if item.GetMedia().GetPredicateName() != mtproto.Predicate_messageMediaVideoStream {
			t.Fatalf("viewer story media %s", item.GetMedia().GetPredicateName())
		}
		assertClientLiveStream(t, item, callID, access, true)
		byID := invokeOK(t, viewer, &mtproto.TLStoriesGetStoriesByID{
			Constructor: mtproto.CRC32_stories_getStoriesByID_5774ca74,
			Peer:        peer,
			Id:          []int32{storyID},
		})
		got := storyFromTL(byID, storyID)
		if got.GetPredicateName() == mtproto.Predicate_storyItemDeleted {
			return false, nil
		}
		assertClientLiveStream(t, got, callID, access, true)
		// recentStory.live is the chat-list ring. Wait until that user object arrives.
		seen, live := viewerRecentLive(ctx, viewer, hostID, peer)
		return seen && live, nil
	}); err != nil {
		t.Fatalf("viewer never read the live stream: %v", err)
	}

	invokeOK(t, host, &mtproto.TLPhoneDiscardGroupCall{
		Constructor: mtproto.CRC32_phone_discardGroupCall,
		Call:        mtproto.MakeTLInputGroupCall(&mtproto.InputGroupCall{Id: callID, AccessHash: access}).To_InputGroupCall(),
	})
	clearCtx, clearCancel := contextTimeout(20 * time.Second)
	defer clearCancel()
	if err := waitUntil(clearCtx, 300*time.Millisecond, func() (bool, error) {
		return !viewerSeesStory(clearCtx, viewer, hostID, peer, storyID), nil
	}); err != nil {
		t.Fatalf("ended stream %d still visible", storyID)
	}
}

func liveCallOf(t *testing.T, obj mtproto.TLObject) (storyID int32, callID, access int64) {
	t.Helper()
	for _, u := range updatesOf(obj) {
		if item := u.GetStory(); item.GetId() != 0 && item.GetPredicateName() != mtproto.Predicate_storyItemDeleted {
			storyID = item.GetId()
		}
		if call := u.GetCall_GROUPCALL(); call.GetId() != 0 {
			callID = call.GetId()
			access = call.GetAccessHash()
		}
	}
	if storyID == 0 || callID == 0 {
		t.Fatalf("startLive story=%d call=%d", storyID, callID)
	}
	return storyID, callID, access
}

func storyFromTL(obj mtproto.TLObject, id int32) *mtproto.StoryItem {
	for _, u := range updatesOf(obj) {
		if item := u.GetStory(); item.GetId() == id {
			return item
		}
	}
	for _, item := range append(peerStoryItems(obj), storiesBoxItems(obj)...) {
		if item.GetId() == id {
			return item
		}
	}
	return nil
}

func storiesBoxItems(obj mtproto.TLObject) []*mtproto.StoryItem {
	switch v := obj.(type) {
	case *mtproto.TLStoriesStories:
		return v.GetStories()
	case *mtproto.Stories_Stories:
		return v.GetStories()
	default:
		return nil
	}
}

func viewerStory(ctx context.Context, viewer *Client, peer *mtproto.InputPeer, storyID int32) (*mtproto.StoryItem, bool) {
	res, err := viewer.Invoke(ctx, &mtproto.TLStoriesGetPeerStories{
		Constructor: mtproto.CRC32_stories_getPeerStories,
		Peer:        peer,
	})
	if err != nil {
		return nil, false
	}
	for _, item := range peerStoryItems(res) {
		if item.GetId() == storyID && item.GetPredicateName() != mtproto.Predicate_storyItemDeleted {
			return item, true
		}
	}
	return nil, false
}

func assertClientLiveStream(t *testing.T, item *mtproto.StoryItem, callID, access int64, rtmp bool) {
	t.Helper()
	if item == nil || item.GetPredicateName() == mtproto.Predicate_storyItemDeleted {
		t.Fatalf("story item %v", item)
	}
	media := item.GetMedia()
	if media.GetPredicateName() != mtproto.Predicate_messageMediaVideoStream {
		t.Fatalf("media %s", media.GetPredicateName())
	}
	call := media.GetCall()
	if call.GetPredicateName() != mtproto.Predicate_inputGroupCall || call.GetId() != callID || call.GetAccessHash() != access || media.GetRtmpStream() != rtmp {
		t.Fatalf("call id=%d access=%d rtmp=%v name=%s", call.GetId(), call.GetAccessHash(), media.GetRtmpStream(), call.GetPredicateName())
	}
}

func assertRecentLiveIfPresent(t *testing.T, obj mtproto.TLObject, host int64) {
	t.Helper()
	user := userByID(usersOf(obj), host)
	recent := user.GetStoriesMaxId_FLAGRECENTSTORY()
	if recent != nil && !recent.GetLive() {
		t.Fatal("recentStory was sent without live")
	}
}

func viewerRecentLive(ctx context.Context, viewer *Client, host int64, peer *mtproto.InputPeer) (seen, live bool) {
	res, err := viewer.Invoke(ctx, &mtproto.TLUsersGetUsers{
		Constructor: mtproto.CRC32_users_getUsers,
		Id:          []*mtproto.InputUser{inputUser(peer.GetUserId(), peer.GetAccessHash())},
	})
	if err != nil {
		return false, false
	}
	recent := userByID(usersOf(res), host).GetStoriesMaxId_FLAGRECENTSTORY()
	if recent == nil {
		return false, false
	}
	return true, recent.GetLive()
}
