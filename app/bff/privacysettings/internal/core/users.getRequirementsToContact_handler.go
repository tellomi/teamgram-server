package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/privacypremium"
)

// UsersGetRequirementsToContact
// users.getRequirementsToContact#d89a83a3 id:Vector<InputUser> = Vector<RequirementToContact>;
//
// Android constructs this as TL_account.getRequirementsToContact. The constructor
// 0xd89a83a3 is users.getRequirementsToContact. There is no account.getRequirementsToContact
// in layer 232.
//
// https://core.telegram.org/method/users.getRequirementsToContact
// https://core.telegram.org/api/privacy
// One RequirementToContact per non-nil input user, in the same order. A nil
// element is skipped, the same as users.getIsPremiumRequiredToContact.
// requirementToContactPremium is the fully checked result: the viewer is not a
// contact, has no existing private chat, and does not have an unexpired Premium
// subscription.
func (c *PrivacySettingsCore) UsersGetRequirementsToContact(in *mtproto.TLUsersGetRequirementsToContact) (*mtproto.Vector_RequirementToContact, error) {
	if c.MD == nil || c.MD.GetUserId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	var ids []*mtproto.InputUser
	if in != nil {
		ids = in.GetId()
	}
	out := make([]*mtproto.RequirementToContact, 0, len(ids))
	for _, id := range ids {
		if id == nil {
			continue
		}
		blocked, err := c.premiumRequired(id)
		if err != nil {
			return nil, err
		}
		out = append(out, privacypremium.Requirement(blocked))
	}
	return &mtproto.Vector_RequirementToContact{Datas: out}, nil
}

func (c *PrivacySettingsCore) premiumRequired(id *mtproto.InputUser) (bool, error) {
	// FromInputUser reads id.PredicateName. A nil element must not reach it.
	if id == nil || c == nil || c.MD == nil || c.svcCtx == nil || c.svcCtx.Dao == nil || c.svcCtx.Dao.UserClient == nil {
		return false, nil
	}
	peer := mtproto.FromInputUser(c.MD.UserId, id)
	if peer.PeerType != mtproto.PEER_USER && peer.PeerType != mtproto.PEER_SELF {
		return false, nil
	}
	verdict, err := privacypremium.Evaluate(c.ctx, c.svcCtx.Dao.UserClient, c.svcCtx.Dao.DialogClient, c.MD.UserId, peer.PeerId)
	if err != nil {
		return false, err
	}
	return verdict.Blocked, nil
}
