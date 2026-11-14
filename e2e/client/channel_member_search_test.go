package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

// TestChannelParticipantSearchByName is ISSUE-S-MEMBER-SEARCH-001 (row 161).
// Phones 9996610457–458, code 11111.
func TestChannelParticipantSearchByName(t *testing.T) {
	cfg := requireE2E(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	owner, _ := dialPhone(t, ctx, cfg, "9996610457", "Own457")
	member, _ := dialPhone(t, ctx, cfg, "9996610458", "Mem458")
	if _, err := member.Invoke(ctx, &mtproto.TLAccountUpdateProfile{
		Constructor: mtproto.CRC32_account_updateProfile,
		FirstName:   wrapperspb.String("Searchling"),
		LastName:    wrapperspb.String("Finder"),
	}); err != nil {
		t.Fatal(err)
	}
	memberID, memberHash, err := owner.ImportPhone(ctx, "9996610458", "Searchling")
	if err != nil {
		t.Fatal(err)
	}
	groupID, groupHash, err := owner.CreateMegagroup(ctx, "FindRoom")
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.InviteToChannel(ctx, inputChannel(groupID, groupHash), inputUser(memberID, memberHash)); err != nil {
		t.Fatal(err)
	}
	got, err := owner.Invoke(ctx, &mtproto.TLChannelsGetParticipants{
		Constructor: mtproto.CRC32_channels_getParticipants,
		Channel:     inputChannel(groupID, groupHash),
		Filter:      mtproto.MakeTLChannelParticipantsSearch(&mtproto.ChannelParticipantsFilter{Q_STRING: "searchling"}).To_ChannelParticipantsFilter(),
		Limit:       50,
	})
	if err != nil {
		t.Fatal(err)
	}
	parts := participantRows(t, got)
	if len(parts) != 1 || parts[0].GetUserId() != memberID {
		t.Fatalf("search %+v", parts)
	}
}

func participantRows(t *testing.T, obj mtproto.TLObject) []*mtproto.ChannelParticipant {
	t.Helper()
	switch v := obj.(type) {
	case *mtproto.TLChannelsChannelParticipants:
		return v.To_Channels_ChannelParticipants().GetParticipants()
	case *mtproto.Channels_ChannelParticipants:
		return v.GetParticipants()
	default:
		t.Fatalf("participants %T", obj)
		return nil
	}
}
