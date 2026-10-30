package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/accountrest"
)

// UsersGetRequirementsToContact
// users.getRequirementsToContact#d89a83a3 id:Vector<InputUser> = Vector<RequirementToContact>;
//
// Android constructs this as TL_account.getRequirementsToContact. The constructor
// 0xd89a83a3 is users.getRequirementsToContact. There is no account.getRequirementsToContact
// in layer 232.
func (c *PrivacySettingsCore) UsersGetRequirementsToContact(in *mtproto.TLUsersGetRequirementsToContact) (*mtproto.Vector_RequirementToContact, error) {
	if c.MD == nil || c.MD.GetUserId() == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	var ids []*mtproto.InputUser
	if in != nil {
		ids = in.GetId()
	}
	return accountrest.RequirementsToContact(ids), nil
}
