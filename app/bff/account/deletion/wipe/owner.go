package wipe

import "github.com/teamgram/proto/mtproto"

// Member is one current participant used to pick the next owner.
type Member struct {
	UserID int64
	Role   int32
	Date   int32
	State  int32
}

// PickSuccessor chooses the next basic-group or channel owner.
// The earliest remaining administrator wins. With no administrator, the
// earliest remaining ordinary member wins. The same rule is used for
// channels in app/service/biz/channel. A creator cannot simply leave:
// Teamgram rejects that with USER_CREATOR, and the transfer RPC requires
// the cloud password, which account deletion cannot ask for. Transferring
// keeps the chat administrable. False means the user is the only member;
// the chat is deactivated and its messages are kept.
func PickSuccessor(members []Member, leaving int64) (int64, bool) {
	var adminID, memberID int64
	var adminDate, memberDate int32
	adminSet, memberSet := false, false
	for _, m := range members {
		if m.UserID == 0 || m.UserID == leaving || m.State != mtproto.ChatMemberStateNormal {
			continue
		}
		switch m.Role {
		case mtproto.ChatMemberAdmin:
			if !adminSet || m.Date < adminDate || (m.Date == adminDate && m.UserID < adminID) {
				adminSet = true
				adminID = m.UserID
				adminDate = m.Date
			}
		case mtproto.ChatMemberNormal:
			if !memberSet || m.Date < memberDate || (m.Date == memberDate && m.UserID < memberID) {
				memberSet = true
				memberID = m.UserID
				memberDate = m.Date
			}
		}
	}
	if adminSet {
		return adminID, true
	}
	if memberSet {
		return memberID, true
	}
	return 0, false
}
