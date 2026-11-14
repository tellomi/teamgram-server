package privacypremium

import (
	"context"
	"testing"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

type fakeUsers struct {
	require  map[int64]bool
	contacts map[int64]map[int64]bool // owner -> contact
	premium  map[int64]bool
}

func (f fakeUsers) UserGetGlobalPrivacySettings(_ context.Context, in *user.TLUserGetGlobalPrivacySettings) (*mtproto.GlobalPrivacySettings, error) {
	return mtproto.MakeTLGlobalPrivacySettings(&mtproto.GlobalPrivacySettings{
		NewNoncontactPeersRequirePremium: f.require[in.GetUserId()],
	}).To_GlobalPrivacySettings(), nil
}

func (f fakeUsers) UserCheckContact(_ context.Context, in *user.TLUserCheckContact) (*mtproto.Bool, error) {
	return mtproto.ToBool(f.contacts[in.GetUserId()][in.GetId()]), nil
}

func (f fakeUsers) UserGetImmutableUser(_ context.Context, in *user.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error) {
	return &mtproto.ImmutableUser{User: &mtproto.UserData{
		Id:                in.GetId(),
		Premium:           f.premium[in.GetId()],
		PremiumExpireDate: &wrapperspb.Int64Value{Value: 0},
	}}, nil
}

type fakeDialogs struct {
	top map[[2]int64]int32
}

func (f fakeDialogs) DialogGetTopMessage(_ context.Context, in *dialog.TLDialogGetTopMessage) (*mtproto.Int32, error) {
	return &mtproto.Int32{V: f.top[[2]int64{in.GetUserId(), in.GetPeerId()}]}, nil
}

func TestEvaluatePremiumRequirement(t *testing.T) {
	users := fakeUsers{
		require:  map[int64]bool{2: true},
		contacts: map[int64]map[int64]bool{2: {4: true}},
		premium:  map[int64]bool{5: true},
	}
	dialogs := fakeDialogs{top: map[[2]int64]int32{{1, 2}: 8}}

	stranger, err := Evaluate(context.Background(), users, dialogs, 3, 2)
	if err != nil || !stranger.Setting || !stranger.Blocked {
		t.Fatalf("stranger %+v %v", stranger, err)
	}
	if Requirement(stranger.Blocked).GetPredicateName() != mtproto.Predicate_requirementToContactPremium || !mtproto.FromBool(Bool(true)) {
		t.Fatal("constructors")
	}

	// An existing private chat is exempt. The raw flag stays set.
	chatting, err := Evaluate(context.Background(), users, dialogs, 1, 2)
	if err != nil || !chatting.Setting || chatting.Blocked {
		t.Fatalf("existing chat %+v %v", chatting, err)
	}
	contact, err := Evaluate(context.Background(), users, fakeDialogs{}, 4, 2)
	if err != nil || !contact.Setting || contact.Blocked {
		t.Fatalf("contact %+v %v", contact, err)
	}
	member, err := Evaluate(context.Background(), users, fakeDialogs{}, 5, 2)
	if err != nil || !member.Setting || member.Blocked {
		t.Fatalf("premium %+v %v", member, err)
	}
	self, err := Evaluate(context.Background(), users, fakeDialogs{}, 2, 2)
	if err != nil || self.Setting || self.Blocked {
		t.Fatalf("self %+v %v", self, err)
	}
	open, err := Evaluate(context.Background(), users, fakeDialogs{}, 3, 9)
	if err != nil || open.Setting || open.Blocked {
		t.Fatalf("setting off %+v %v", open, err)
	}
	if err := RejectSend(context.Background(), users, fakeDialogs{}, 3, 2); err != mtproto.ErrPrivacyPremiumRequired {
		t.Fatalf("reject %v", err)
	}
	if err := RejectSend(context.Background(), users, dialogs, 1, 2); err != nil {
		t.Fatal(err)
	}
}
