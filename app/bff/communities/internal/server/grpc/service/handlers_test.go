package service

import (
	"context"
	"encoding/base64"
	"testing"

	"github.com/teamgram/proto/mtproto"
	rpcmd "github.com/teamgram/proto/mtproto/rpc/metadata"
	"github.com/teamgram/teamgram-server/app/bff/communities/internal/book"
	"github.com/teamgram/teamgram-server/app/bff/communities/internal/svc"

	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func withUser(userID int64) context.Context {
	md := &rpcmd.RpcMetadata{UserId: userID, PermAuthKeyId: userID + 10}
	buf, _ := proto.Marshal(md)
	return metadata.NewIncomingContext(context.Background(), metadata.Pairs(
		"rpc_metadata", base64.StdEncoding.EncodeToString(buf),
	))
}

func errText(err error) string {
	if err == nil {
		return ""
	}
	return status.Convert(err).Message()
}

func newSvc() *Service {
	return New(&svc.ServiceContext{Book: book.New()})
}

func mustEncode(t *testing.T, obj interface {
	Encode(*mtproto.EncodeBuf, int32) error
}) {
	t.Helper()
	buf := mtproto.NewEncodeBuf(2048)
	if err := obj.Encode(buf, 232); err != nil {
		t.Fatal(err)
	}
}

func inputChannel(ch *mtproto.Chat) *mtproto.InputChannel {
	return mtproto.MakeTLInputChannel(&mtproto.InputChannel{
		ChannelId:  ch.GetId(),
		AccessHash: ch.GetAccessHash_FLAGINT64().GetValue(),
	}).To_InputChannel()
}

func TestCommunities(t *testing.T) {
	svc := newSvc()
	owner := withUser(42)
	other := withUser(43)
	peer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 77, AccessHash: 1}).To_InputPeer()
	ups, err := svc.CommunitiesCreate(owner, &mtproto.TLCommunitiesCreate{
		Title:  "Block",
		About:  wrapperspb.String("neighbors"),
		Hidden: false,
		Peer:   peer,
	})
	if err != nil {
		t.Fatal(err)
	}
	mustEncode(t, ups)
	if len(ups.GetUpdates()) != 1 || ups.GetUpdates()[0].GetPredicateName() != mtproto.Predicate_updateChannel || ups.GetSeq() != 0 {
		t.Fatalf("updates %#v", ups)
	}
	if len(ups.GetChats()) != 1 || ups.GetChats()[0].GetPredicateName() != mtproto.Predicate_community {
		t.Fatalf("chats %#v", ups.GetChats())
	}
	ch := inputChannel(ups.GetChats()[0])
	if _, err := svc.CommunitiesCreate(owner, &mtproto.TLCommunitiesCreate{Title: "  ", Peer: peer}); errText(err) != "CHAT_TITLE_EMPTY" {
		t.Fatal(err)
	}

	joined, err := svc.CommunitiesGetJoinedCommunities(owner, &mtproto.TLCommunitiesGetJoinedCommunities{})
	if err != nil || len(joined.GetChats()) != 1 {
		t.Fatalf("joined %#v %v", joined, err)
	}
	mustEncode(t, joined)
	empty, err := svc.CommunitiesGetJoinedCommunities(other, &mtproto.TLCommunitiesGetJoinedCommunities{})
	if err != nil || len(empty.GetChats()) != 0 {
		t.Fatalf("other joined %#v %v", empty, err)
	}

	reqPeer := mtproto.MakeTLInputPeerChannel(&mtproto.InputPeer{ChannelId: 88, AccessHash: 1}).To_InputPeer()
	if _, err := svc.CommunitiesTogglePeerLink(other, &mtproto.TLCommunitiesTogglePeerLink{
		Visible:   true,
		Community: ch,
		Peer:      reqPeer,
	}); err != nil {
		t.Fatal(err)
	}
	page, err := svc.CommunitiesGetPeerLinkRequests(owner, &mtproto.TLCommunitiesGetPeerLinkRequests{Community: ch, Limit: 10})
	if err != nil || page.GetTotalCount() != 1 || page.GetRequests()[0].GetRequestedBy() != 43 {
		t.Fatalf("requests %#v %v", page, err)
	}
	mustEncode(t, page)
	if _, err := svc.CommunitiesGetPeerLinkRequests(other, &mtproto.TLCommunitiesGetPeerLinkRequests{Community: ch}); errText(err) != "CHAT_ADMIN_REQUIRED" {
		t.Fatal(err)
	}
	if _, err := svc.CommunitiesTogglePeerLinkRequestApproval(owner, &mtproto.TLCommunitiesTogglePeerLinkRequestApproval{
		Community: ch,
		Peer:      reqPeer,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CommunitiesToggleAllPeerLinkRequestApproval(owner, &mtproto.TLCommunitiesToggleAllPeerLinkRequestApproval{
		Reject:    true,
		Community: ch,
	}); err != nil {
		t.Fatal(err)
	}

	chats, err := svc.CommunitiesGetParticipantJoinedChats(owner, &mtproto.TLCommunitiesGetParticipantJoinedChats{
		Community:   ch,
		Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 42}).To_InputPeer(),
	})
	if err != nil || len(chats.GetCreatorChatIds()) != 1 || chats.GetCreatorChatIds()[0] != 77 {
		t.Fatalf("joined chats %#v %v", chats, err)
	}
	mustEncode(t, chats)

	collapsed, err := svc.CommunitiesToggleCommunityCollapsedInDialogs(owner, &mtproto.TLCommunitiesToggleCommunityCollapsedInDialogs{
		Collapsed: true,
		Community: ch,
	})
	if err != nil || !collapsed.GetChats()[0].GetCollapsedInDialogs() || collapsed.GetSeq() != 0 {
		t.Fatalf("collapse %#v %v", collapsed, err)
	}
	mustEncode(t, collapsed)

	if _, err := svc.CommunitiesToggleParticipantBanned(owner, &mtproto.TLCommunitiesToggleParticipantBanned{
		Community:   ch,
		Participant: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	}); errText(err) != "USER_CREATOR" {
		t.Fatal(err)
	}
	if _, err := svc.CommunitiesToggleParticipantBanned(owner, &mtproto.TLCommunitiesToggleParticipantBanned{
		Community:   ch,
		Participant: mtproto.MakeTLInputPeerUser(&mtproto.InputPeer{UserId: 43}).To_InputPeer(),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CommunitiesGetParticipantJoinedChats(other, &mtproto.TLCommunitiesGetParticipantJoinedChats{
		Community:   ch,
		Participant: mtproto.MakeTLInputPeerSelf(nil).To_InputPeer(),
	}); errText(err) != "USER_BANNED_IN_CHANNEL" {
		t.Fatal(err)
	}
	if _, err := svc.CommunitiesTogglePeerLink(owner, &mtproto.TLCommunitiesTogglePeerLink{
		Deleted:   true,
		Community: ch,
		Peer:      peer,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CommunitiesGetPeerLinkRequests(owner, &mtproto.TLCommunitiesGetPeerLinkRequests{Community: ch, Offset: "nope"}); errText(err) != "OFFSET_INVALID" {
		t.Fatal(err)
	}
}
