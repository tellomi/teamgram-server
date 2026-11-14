package core

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	userclient "github.com/teamgram/teamgram-server/app/service/biz/user/client"
	userpb "github.com/teamgram/teamgram-server/app/service/biz/user/user"

	"google.golang.org/grpc/status"
)

func TestReservedBotClaimRejectsNewNames(t *testing.T) {
	c := usernameCore(t, occupyResolver{err: mtproto.ErrUsernameNotOccupied})
	for _, name := range []string{"bing", "BotFather", "WIKI", "imdb", "bold", "music", "sticker", "venue"} {
		err := c.rejectReservedBotClaim(name, mtproto.PEER_USER, 7)
		if status.Convert(err).Message() != "USERNAME_OCCUPIED" {
			t.Fatalf("%s: %v", name, err)
		}
		_, err = c.AccountCheckUsername(&mtproto.TLAccountCheckUsername{Username: name})
		if status.Convert(err).Message() != "USERNAME_OCCUPIED" {
			t.Fatalf("check %s: %v", name, err)
		}
		if err = c.updateUsername(7, "", name); status.Convert(err).Message() != "USERNAME_OCCUPIED" {
			t.Fatalf("update %s: %v", name, err)
		}
	}
	_, err := c.ChannelsCheckUsername(&mtproto.TLChannelsCheckUsername{
		Channel:  mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 9}).To_InputChannel(),
		Username: "bing",
	})
	if status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("channel check: %v", err)
	}
	_, err = c.ChannelsUpdateUsername(&mtproto.TLChannelsUpdateUsername{
		Channel:  mtproto.MakeTLInputChannel(&mtproto.InputChannel{ChannelId: 9}).To_InputChannel(),
		Username: "BotFather",
	})
	if status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("channel update: %v", err)
	}
}

func TestReservedBotClaimKeepsTheHolder(t *testing.T) {
	c := usernameCore(t, occupyResolver{peer: mtproto.MakePeerUser(7)})
	if err := c.rejectReservedBotClaim("bing", mtproto.PEER_USER, 7); err != nil {
		t.Fatal(err)
	}
	if err := c.rejectReservedBotClaim("bing", mtproto.PEER_USER, 8); status.Convert(err).Message() != "USERNAME_OCCUPIED" {
		t.Fatalf("other user: %v", err)
	}
	c = usernameCore(t, occupyResolver{peer: mtproto.MakePeerChannel(9)})
	if err := c.rejectReservedBotClaim("wiki", mtproto.PEER_CHANNEL, 9); err != nil {
		t.Fatal(err)
	}
}

func TestReservedBotClaimDoesNotTreatLookupFailureAsFree(t *testing.T) {
	c := usernameCore(t, occupyResolver{err: mtproto.ErrInternalServerError})
	err := c.rejectReservedBotClaim("bing", mtproto.PEER_USER, 7)
	if err == nil || status.Convert(err).Message() == "USERNAME_OCCUPIED" {
		t.Fatalf("lookup failure: %v", err)
	}
	reached := false
	func() {
		defer func() {
			if recover() != nil {
				reached = true
			}
		}()
		_ = c.updateUsername(7, "", "roomname")
	}()
	if !reached {
		t.Fatal("an ordinary name must still reach the user service")
	}
}

type occupyResolver struct {
	userclient.UserClient
	peer *mtproto.Peer
	err  error
}

func (o occupyResolver) UserResolveUsername(context.Context, *userpb.TLUserResolveUsername) (*mtproto.Peer, error) {
	if o.err != nil {
		return nil, o.err
	}
	return o.peer, nil
}
