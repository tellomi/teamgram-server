package push

// Device token types from account.registerDevice.
// https://core.telegram.org/api/push-updates
const (
	TokenAPNS       int32 = 1
	TokenFCM        int32 = 2
	TokenMPNS       int32 = 3
	TokenSimplePush int32 = 4
	TokenUbuntu     int32 = 5
	TokenBlackberry int32 = 6
	TokenMTProto    int32 = 7
	TokenWNS        int32 = 8
	TokenAPNSVoIP   int32 = 9
	TokenWeb        int32 = 10
	TokenMPNSVoIP   int32 = 11
	TokenTizen      int32 = 12
	TokenHuawei     int32 = 13
)
