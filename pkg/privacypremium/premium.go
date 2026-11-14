// Package privacypremium applies globalPrivacySettings.new_noncontact_peers_require_premium.
//
// https://core.telegram.org/api/privacy
// https://core.telegram.org/method/users.getRequirementsToContact
//
// user.contact_require_premium is the setting itself, shown to other users
// even when they can still write (a contact, an existing private chat, or a
// Premium subscriber). userFull.contact_require_premium and
// users.getRequirementsToContact are the fully checked result: the viewer
// cannot open a new private chat.
//
// A sender without an unexpired users.premium subscription who is not in the
// recipient's contacts and has no existing private chat receives
// 403 PRIVACY_PREMIUM_REQUIRED. TELLOMI_PREMIUM_EVERYONE makes every account
// look Premium for feature limits and does not waive this, matching
// messages.getOutboxReadDate.
package privacypremium

import (
	"context"
	"time"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/service/biz/dialog/dialog"
	"github.com/teamgram/teamgram-server/app/service/biz/user/user"
	"github.com/teamgram/teamgram-server/pkg/premium"
)

// Settings is the user-service surface this check needs.
type Settings interface {
	UserGetGlobalPrivacySettings(ctx context.Context, in *user.TLUserGetGlobalPrivacySettings) (*mtproto.GlobalPrivacySettings, error)
	UserCheckContact(ctx context.Context, in *user.TLUserCheckContact) (*mtproto.Bool, error)
	UserGetImmutableUser(ctx context.Context, in *user.TLUserGetImmutableUser) (*mtproto.ImmutableUser, error)
}

// Dialogs reports whether a private dialog already has a message.
type Dialogs interface {
	DialogGetTopMessage(ctx context.Context, in *dialog.TLDialogGetTopMessage) (*mtproto.Int32, error)
}

// Verdict is one target seen by one viewer.
type Verdict struct {
	// Setting is new_noncontact_peers_require_premium. It is user.contact_require_premium.
	Setting bool
	// Blocked is the fully checked requirement. It is userFull.contact_require_premium
	// and requirementToContactPremium.
	Blocked bool
}

// Evaluate applies the layer-232 rule for viewer writing to target.
// viewer == target is never blocked. A missing settings row is not an error.
func Evaluate(ctx context.Context, users Settings, dialogs Dialogs, viewer, target int64) (Verdict, error) {
	if users == nil || viewer == 0 || target == 0 || viewer == target {
		return Verdict{}, nil
	}
	settings, err := users.UserGetGlobalPrivacySettings(ctx, &user.TLUserGetGlobalPrivacySettings{UserId: target})
	if err != nil {
		return Verdict{}, err
	}
	if settings == nil || !settings.GetNewNoncontactPeersRequirePremium() {
		return Verdict{}, nil
	}
	out := Verdict{Setting: true}
	paid, err := paidPremium(ctx, users, viewer)
	if err != nil {
		return Verdict{}, err
	}
	if paid {
		return out, nil
	}
	contact, err := users.UserCheckContact(ctx, &user.TLUserCheckContact{UserId: target, Id: viewer})
	if err != nil {
		return Verdict{}, err
	}
	if mtproto.FromBool(contact) {
		return out, nil
	}
	if hasPrivateChat(ctx, dialogs, viewer, target) {
		return out, nil
	}
	out.Blocked = true
	return out, nil
}

// RejectSend is the messages.* private-chat gate.
// 403 PRIVACY_PREMIUM_REQUIRED when Evaluate says the viewer cannot write.
func RejectSend(ctx context.Context, users Settings, dialogs Dialogs, viewer, target int64) error {
	verdict, err := Evaluate(ctx, users, dialogs, viewer, target)
	if err != nil {
		return err
	}
	if verdict.Blocked {
		return mtproto.ErrPrivacyPremiumRequired
	}
	return nil
}

// Requirement is one users.getRequirementsToContact element.
func Requirement(blocked bool) *mtproto.RequirementToContact {
	if blocked {
		return mtproto.MakeTLRequirementToContactPremium(&mtproto.RequirementToContact{}).To_RequirementToContact()
	}
	return mtproto.MakeTLRequirementToContactEmpty(&mtproto.RequirementToContact{}).To_RequirementToContact()
}

// Bool is one users.getIsPremiumRequiredToContact element.
func Bool(blocked bool) *mtproto.Bool {
	if blocked {
		return mtproto.BoolTrue
	}
	return mtproto.BoolFalse
}

func paidPremium(ctx context.Context, users Settings, viewer int64) (bool, error) {
	me, err := users.UserGetImmutableUser(ctx, &user.TLUserGetImmutableUser{Id: viewer})
	if err != nil || me == nil || me.User == nil || me.Deleted() {
		return false, err
	}
	var expire int64
	if me.User.GetPremiumExpireDate() != nil {
		expire = me.User.GetPremiumExpireDate().GetValue()
	}
	return premium.ActiveSubscription(me.User.GetPremium(), expire, time.Now().Unix()), nil
}

func hasPrivateChat(ctx context.Context, dialogs Dialogs, a, b int64) bool {
	if dialogs == nil {
		return false
	}
	return topMessage(ctx, dialogs, a, b) > 0 || topMessage(ctx, dialogs, b, a) > 0
}

func topMessage(ctx context.Context, dialogs Dialogs, userID, peerID int64) int32 {
	top, err := dialogs.DialogGetTopMessage(ctx, &dialog.TLDialogGetTopMessage{
		UserId:   userID,
		PeerType: mtproto.PEER_USER,
		PeerId:   peerID,
	})
	if err != nil || top == nil {
		return 0
	}
	return top.GetV()
}
