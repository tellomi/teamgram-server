package mtproto

import "testing"

func TestToUserIdByInputResolvesAllowListUsers(t *testing.T) {
	self := int64(1)
	cases := []struct {
		name string
		in   *InputUser
		want int64
	}{
		{"nil", nil, 0},
		{"empty", MakeTLInputUserEmpty(nil).To_InputUser(), 0},
		{"self", MakeTLInputUserSelf(nil).To_InputUser(), self},
		{"inputUser", MakeTLInputUser(&InputUser{UserId: 2, AccessHash: 9}).To_InputUser(), 2},
		{"fromMessage", MakeTLInputUserFromMessage(&InputUser{
			UserId: 2,
			MsgId:  4,
			Peer:   MakeTLInputPeerUser(&InputPeer{UserId: 8, AccessHash: 1}).To_InputPeer(),
		}).To_InputUser(), 2},
		{"constructor only", &InputUser{Constructor: CRC32_inputUser, UserId: 2, AccessHash: 9}, 2},
		{"id only", &InputUser{UserId: 2}, 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ToUserIdByInput(self, tc.in); got != tc.want {
				t.Fatalf("got %d want %d", got, tc.want)
			}
		})
	}
	ids := ToUserIdListByInput(self, []*InputUser{
		nil,
		MakeTLInputUserEmpty(nil).To_InputUser(),
		MakeTLInputUserFromMessage(&InputUser{UserId: 2, MsgId: 1}).To_InputUser(),
		&InputUser{UserId: 3},
	})
	if len(ids) != 2 || ids[0] != 2 || ids[1] != 3 {
		t.Fatalf("list %v", ids)
	}
}
