package service

import (
	"context"
	"errors"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/messenger/sync/sync"
	"github.com/teamgram/teamgram-server/app/service/biz/chatbox"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
)

func TestPrivateReactionBlockedByPeerReturnsUserIsBlocked(t *testing.T) {
	chatbox.ResetShared()
	ctx := withReactionUser(7, false)
	push := &reactionPush{}
	svc := ReactionsService{Blocks: reactionBlocks{peerBlockedSelf: true}, Push: push}
	peer := privatePeer(8)
	if _, err := svc.MessagesSendReaction(ctx, thumbsUp(peer, 4)); err != mtproto.ErrUserIsBlocked {
		t.Fatalf("blocked party: %v", err)
	}
	if push.n != 0 {
		t.Fatalf("blocked reaction was pushed %d", push.n)
	}
	if stored := reactionStored(ctx, 7, 8, 4); stored {
		t.Fatal("reaction was stored after USER_IS_BLOCKED")
	}
}

func TestPrivateReactionWhenSenderBlockedThePeer(t *testing.T) {
	chatbox.ResetShared()
	ctx := withReactionUser(7, false)
	push := &reactionPush{}
	svc := ReactionsService{Blocks: reactionBlocks{selfBlockedPeer: true}, Push: push}
	if _, err := svc.MessagesSendReaction(ctx, thumbsUp(privatePeer(8), 4)); err != mtproto.ErrYouBlockedUser {
		t.Fatalf("sender blocked the peer: %v", err)
	}
	if push.n != 0 || reactionStored(ctx, 7, 8, 4) {
		t.Fatal("reaction was stored or pushed after YOU_BLOCKED_USER")
	}
}

func TestPrivateReactionMutualBlockReportsUserIsBlocked(t *testing.T) {
	ctx := withReactionUser(7, false)
	svc := ReactionsService{Blocks: reactionBlocks{peerBlockedSelf: true, selfBlockedPeer: true}}
	if _, err := svc.MessagesSendReaction(ctx, thumbsUp(privatePeer(8), 4)); err != mtproto.ErrUserIsBlocked {
		t.Fatalf("mutual: %v", err)
	}
}

func TestPrivateReactionAllowedWhenNotBlocked(t *testing.T) {
	chatbox.ResetShared()
	ctx := withReactionUser(7, false)
	push := &reactionPush{}
	svc := ReactionsService{Blocks: reactionBlocks{}, Push: push}
	up, err := svc.MessagesSendReaction(ctx, thumbsUp(privatePeer(8), 4))
	if err != nil || up == nil || len(up.Updates) == 0 {
		t.Fatalf("open chat: %#v %v", up, err)
	}
	if !reactionStored(ctx, 7, 8, 4) {
		t.Fatal("allowed reaction was not stored")
	}
}

func TestPrivateReactionLookupErrorDoesNotBlock(t *testing.T) {
	chatbox.ResetShared()
	ctx := withReactionUser(7, false)
	svc := ReactionsService{Blocks: reactionBlocks{err: errors.New("user rpc down")}}
	if _, err := svc.MessagesSendReaction(ctx, thumbsUp(privatePeer(8), 4)); err != nil {
		t.Fatalf("lookup error: %v", err)
	}
}

func TestSavedMessagesReactionIgnoresABlock(t *testing.T) {
	if err := privateReactionBlocked(context.Background(), reactionBlocks{peerBlockedSelf: true}, 7, mtproto.MakePeerUtil(mtproto.PEER_USER, 7)); err != nil {
		t.Fatal(err)
	}
	if err := privateReactionBlocked(context.Background(), reactionBlocks{peerBlockedSelf: true}, 7, mtproto.MakePeerUtil(mtproto.PEER_CHAT, 8)); err != nil {
		t.Fatal(err)
	}
}

func privatePeer(id int64) *mtproto.InputPeer {
	return mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: id}).To_InputPeer()
}

func thumbsUp(peer *mtproto.InputPeer, msgID int32) *mtproto.TLMessagesSendReaction {
	return &mtproto.TLMessagesSendReaction{
		Peer:  peer,
		MsgId: msgID,
		Reaction_FLAGVECTORREACTION: []*mtproto.Reaction{
			mtproto.MakeTLReactionEmoji(&mtproto.Reaction{Emoticon: "👍"}).To_Reaction(),
		},
	}
}

func reactionStored(ctx context.Context, self, peerID int64, msgID int32) bool {
	up, err := chatbox.Shared().ReactionsFor(ctx, self, mtproto.MakePeerUtil(mtproto.PEER_USER, peerID), []int32{msgID})
	if err != nil || up == nil {
		return false
	}
	for _, u := range up.Updates {
		for _, row := range u.GetReactions_MESSAGEREACTIONS().GetResults() {
			if row.GetReaction_REACTION().GetEmoticon() == "👍" || row.GetCount() > 0 {
				return true
			}
		}
	}
	return false
}

type reactionBlocks struct {
	peerBlockedSelf bool
	selfBlockedPeer bool
	err             error
}

func (b reactionBlocks) UserIsBlockedByUser(_ context.Context, in *user.TLUserIsBlockedByUser) (*mtproto.Bool, error) {
	if b.err != nil {
		return nil, b.err
	}
	if in.GetUserId() == 8 && in.GetPeerUserId() == 7 && b.peerBlockedSelf {
		return mtproto.BoolTrue, nil
	}
	if in.GetUserId() == 7 && in.GetPeerUserId() == 8 && b.selfBlockedPeer {
		return mtproto.BoolTrue, nil
	}
	return mtproto.BoolFalse, nil
}

type reactionPush struct{ n int }

func (p *reactionPush) SyncPushUpdates(context.Context, *sync.TLSyncPushUpdates) (*mtproto.Void, error) {
	p.n++
	return mtproto.EmptyVoid, nil
}

func (p *reactionPush) SyncUpdatesNotMe(context.Context, *sync.TLSyncUpdatesNotMe) (*mtproto.Void, error) {
	return mtproto.EmptyVoid, nil
}
