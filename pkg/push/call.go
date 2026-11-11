package push

import (
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/teamgram/proto/mtproto"
)

const (
	locKeyPhoneCallRequest = "PHONE_CALL_REQUEST"
	// Apple's VoIP payload limit. A larger body is rejected and never rings.
	voipPayloadLimit   = 5120
	updatesConstructor = 0x74ae4240 // updates#74ae4240
)

// Call is one incoming phoneCallRequested for a callee.
// Updates is the same updates# object sync already fans out, TL-encoded into
// custom.updates. Telegram-iOS PushKit decrypts "p" and will only report
// CallKit when that blob parses as updates containing phoneCallRequested and
// a user whose id is admin_id.
type Call struct {
	UserID     int64 // callee, written as user_id
	CallID     int64
	AccessHash int64
	CallerID   int64
	CallerName string
	Updates    *mtproto.Updates
}

// ShouldDeliver reports whether sync should hand these updates to Service.Deliver.
// Message alerts and an incoming call both qualify. A phoneCallWaiting sent to
// the caller does not.
func ShouldDeliver(userID int64, updates *mtproto.Updates) bool {
	if userID == 0 || updates == nil {
		return false
	}
	if len(Messages(userID, updates)) > 0 {
		return true
	}
	return phoneRequested(updates) != nil
}

// prepareCall builds a VoIP push for phoneCallRequested.
// skip is set when a request exists but sending it would wake PushKit without
// a payload the client can turn into CallKit (missing caller user, or no protocol).
func prepareCall(userID int64, updates *mtproto.Updates) (call Call, skip string) {
	req := phoneRequested(updates)
	if req == nil {
		return Call{}, ""
	}
	if req.GetId() == 0 || req.GetAdminId() == 0 {
		return Call{}, "phoneCallRequested is missing id"
	}
	if req.GetProtocol() == nil {
		return Call{}, "phoneCallRequested is missing protocol"
	}
	caller := callerUser(updates, req.GetAdminId())
	if caller == nil {
		return Call{}, fmt.Sprintf("phoneCallRequested call=%d has no caller user %d", req.GetId(), req.GetAdminId())
	}
	name := joinName(caller.GetFirstName().GetValue(), caller.GetLastName().GetValue())
	if name == "" {
		name = "User"
	}
	return Call{
		UserID:     userID,
		CallID:     req.GetId(),
		AccessHash: req.GetAccessHash(),
		CallerID:   req.GetAdminId(),
		CallerName: name,
		Updates:    updates,
	}, ""
}

func phoneRequested(updates *mtproto.Updates) *mtproto.PhoneCall {
	if updates == nil {
		return nil
	}
	var found *mtproto.PhoneCall
	mtproto.VisitUpdates(0, updates, map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updatePhoneCall: func(_ int64, update *mtproto.Update, _ []*mtproto.User, _ []*mtproto.Chat, _ int32) {
			call := update.GetPhoneCall()
			if found != nil || call == nil {
				return
			}
			if call.GetPredicateName() == mtproto.Predicate_phoneCallRequested {
				found = call
			}
		},
	})
	return found
}

func callerUser(updates *mtproto.Updates, adminID int64) *mtproto.User {
	if updates == nil || adminID == 0 {
		return nil
	}
	for _, u := range updates.GetUsers() {
		if u != nil && u.GetId() == adminID {
			return u
		}
	}
	return nil
}

// BuildCall is the APNs body for one VoIP device.
// secret is the registerDevice secret (the 256-byte master notification key).
// Without it the client cannot decrypt "p" and returns from PushKit without
// reporting CallKit, which is how Apple revokes the VoIP entitlement.
func BuildCall(c Call, secret []byte) ([]byte, error) {
	if c.Updates == nil || c.CallID == 0 || c.CallerID == 0 {
		return nil, fmt.Errorf("incomplete call push")
	}
	if c.Updates.GetPredicateName() != mtproto.Predicate_updates {
		return nil, fmt.Errorf("call push updates must be updates, got %q", c.Updates.GetPredicateName())
	}
	if callerUser(c.Updates, c.CallerID) == nil {
		return nil, fmt.Errorf("call push is missing caller user %d", c.CallerID)
	}
	if len(secret) < 128 {
		return nil, fmt.Errorf("voip push requires the device secret")
	}
	updates := compactCallUpdates(c.Updates, c.CallerID)
	raw, err := encodeUpdates(updates)
	if err != nil {
		return nil, err
	}
	if packed := gzipPack(raw); len(packed) > 0 && len(packed) < len(raw) {
		raw = packed
	}
	updatesB64 := base64.RawURLEncoding.EncodeToString(raw)
	name := c.CallerName
	if name == "" {
		name = "User"
	}
	callID := strconv.FormatInt(c.CallID, 10)
	callAH := strconv.FormatInt(c.AccessHash, 10)
	fromID := strconv.FormatInt(c.CallerID, 10)
	// Telegram-iOS PushKit decrypts "p" and reads updates from that JSON
	// only. call_id / call_ah / from_id stay strings (`as? String`). The
	// official envelope also carries loc-key, aps, and those ids; it does
	// not repeat the updates blob on the outer object or in custom.
	inner := map[string]any{
		"loc-key": locKeyPhoneCallRequest,
		"call_id": callID,
		"call_ah": callAH,
		"from_id": fromID,
		"updates": updatesB64,
		"aps": map[string]any{
			"alert": map[string]any{
				"loc-key":  locKeyPhoneCallRequest,
				"loc-args": []string{name},
			},
		},
	}
	if c.UserID != 0 {
		inner["user_id"] = c.UserID
	}
	innerJSON, err := json.Marshal(inner)
	if err != nil {
		return nil, err
	}
	p, err := EncryptNotification(secret, innerJSON)
	if err != nil {
		return nil, err
	}
	outer := map[string]any{
		"aps": map[string]any{
			"alert": map[string]any{
				"loc-key":  locKeyPhoneCallRequest,
				"loc-args": []string{name},
			},
		},
		"loc-key": locKeyPhoneCallRequest,
		"call_id": callID,
		"call_ah": callAH,
		"from_id": fromID,
		"p":       p,
	}
	if c.UserID != 0 {
		outer["user_id"] = c.UserID
	}
	outerJSON, err := json.Marshal(outer)
	if err != nil {
		return nil, err
	}
	if len(outerJSON) > voipPayloadLimit {
		return nil, fmt.Errorf("voip payload is %d bytes, limit %d", len(outerJSON), voipPayloadLimit)
	}
	return outerJSON, nil
}

// BuildAndroidCall is the FCM data map and the Huawei data string for one
// incoming call. android is a JSON object whose values are strings. huawei
// is the sealed p value alone. The plaintext uses loc_key (underscore), which
// is what the Android listener reads after decrypting p. There is no message
// text in either value. A short secret is refused: the client reads only p.
func BuildAndroidCall(c Call, secret []byte) (android []byte, huawei string, err error) {
	if c.Updates == nil || c.CallID == 0 || c.CallerID == 0 {
		return nil, "", fmt.Errorf("incomplete call push")
	}
	if len(secret) < 128 {
		return nil, "", fmt.Errorf("android call push requires the device secret")
	}
	raw, err := encodeUpdates(c.Updates)
	if err != nil {
		return nil, "", err
	}
	updatesB64 := base64.RawURLEncoding.EncodeToString(raw)
	name := c.CallerName
	if name == "" {
		name = "User"
	}
	callID := strconv.FormatInt(c.CallID, 10)
	callAH := strconv.FormatInt(c.AccessHash, 10)
	fromID := strconv.FormatInt(c.CallerID, 10)
	customObj := map[string]any{
		"call_id": callID,
		"call_ah": callAH,
		"from_id": fromID,
		"updates": updatesB64,
	}
	plain := map[string]any{
		"loc_key":  locKeyPhoneCallRequest,
		"loc_args": []string{name},
		"custom":   customObj,
		"call_id":  callID,
		"call_ah":  callAH,
		"from_id":  fromID,
		"updates":  updatesB64,
	}
	if c.UserID != 0 {
		plain["user_id"] = c.UserID
	}
	plainJSON, err := json.Marshal(plain)
	if err != nil {
		return nil, "", err
	}
	p, err := EncryptNotification(secret, plainJSON)
	if err != nil {
		return nil, "", err
	}
	argsJSON, err := json.Marshal([]string{name})
	if err != nil {
		return nil, "", err
	}
	customJSON, err := json.Marshal(customObj)
	if err != nil {
		return nil, "", err
	}
	data := map[string]string{
		"loc_key":  locKeyPhoneCallRequest,
		"loc_args": string(argsJSON),
		"custom":   string(customJSON),
		"call_id":  callID,
		"call_ah":  callAH,
		"from_id":  fromID,
		"updates":  updatesB64,
		"p":        p,
	}
	if c.UserID != 0 {
		data["user_id"] = strconv.FormatInt(c.UserID, 10)
	}
	out, err := json.Marshal(data)
	if err != nil {
		return nil, "", err
	}
	return out, p, nil
}

const gzipPackedConstructor = 0x3072cfa1 // gzip_packed#3072cfa1

// compactCallUpdates keeps phoneCallRequested and a slim caller user.
// Production requestCall updates also include the callee and full
// user photo/status/color objects. Repeating that blob in the envelope
// and again inside p is what pushed the body over Apple's 5120-byte
// VoIP limit, so PushKit never woke a background callee.
func compactCallUpdates(u *mtproto.Updates, callerID int64) *mtproto.Updates {
	req := phoneRequested(u)
	if req == nil {
		return u
	}
	var callUpdate *mtproto.Update
	mtproto.VisitUpdates(0, u, map[string]mtproto.UpdateVisitedFunc{
		mtproto.Predicate_updatePhoneCall: func(_ int64, update *mtproto.Update, _ []*mtproto.User, _ []*mtproto.Chat, _ int32) {
			if callUpdate != nil || update == nil {
				return
			}
			call := update.GetPhoneCall()
			if call != nil && call.GetId() == req.GetId() && call.GetPredicateName() == mtproto.Predicate_phoneCallRequested {
				callUpdate = update
			}
		},
	})
	if callUpdate == nil {
		return u
	}
	var users []*mtproto.User
	if slim := slimUser(callerUser(u, callerID)); slim != nil {
		users = []*mtproto.User{slim}
	}
	return mtproto.MakeUpdatesByUpdatesUsers(users, callUpdate)
}

func slimUser(u *mtproto.User) *mtproto.User {
	if u == nil {
		return nil
	}
	return mtproto.MakeTLUser(&mtproto.User{
		Id:            u.GetId(),
		AccessHash:    u.AccessHash,
		FirstName:     u.FirstName,
		LastName:      u.LastName,
		Username:      u.Username,
		Phone:         u.Phone,
		Bot:           u.GetBot(),
		Verified:      u.GetVerified(),
		Fake:          u.GetFake(),
		Scam:          u.GetScam(),
		Deleted:       u.GetDeleted(),
		Min:           u.GetMin(),
		Premium:       u.GetPremium(),
		Support:       u.GetSupport(),
		Restricted:    u.GetRestricted(),
		Contact:       u.GetContact(),
		MutualContact: u.GetMutualContact(),
		CloseFriend:   u.GetCloseFriend(),
		LangCode:      u.LangCode,
		Usernames:     u.Usernames,
	}).To_User()
}

func encodeUpdates(u *mtproto.Updates) (raw []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("encode updates: %v", r)
		}
	}()
	buf := mtproto.NewEncodeBuf(1024)
	if err = u.Encode(buf, mtproto.Layer); err != nil {
		return nil, err
	}
	raw = append([]byte(nil), buf.GetBuf()...)
	if len(raw) < 4 {
		return nil, fmt.Errorf("encoded updates too short")
	}
	ctor := binary.LittleEndian.Uint32(raw[:4])
	if ctor != updatesConstructor {
		return nil, fmt.Errorf("encoded updates constructor 0x%x, want updates#74ae4240", ctor)
	}
	return raw, nil
}

// gzipPack wraps TL bytes in gzip_packed#3072cfa1, which
// Telegram-iOS AccountStateManager.extractIncomingCallUpdate unwraps first.
func gzipPack(raw []byte) []byte {
	if len(raw) == 0 {
		return nil
	}
	buf := mtproto.NewEncodeBuf(len(raw) + 64)
	gz := &mtproto.TLGzipPacked{PackedData: raw}
	if err := gz.Encode(buf, mtproto.Layer); err != nil {
		return nil
	}
	out := append([]byte(nil), buf.GetBuf()...)
	if len(out) < 4 || binary.LittleEndian.Uint32(out[:4]) != gzipPackedConstructor {
		return nil
	}
	return out
}

// decodeCallUpdates unwraps an optional gzip_packed envelope and decodes
// updates#74ae4240, matching extractIncomingCallUpdate on Telegram-iOS.
func decodeCallUpdates(raw []byte) (*mtproto.Updates, error) {
	if len(raw) < 4 {
		return nil, fmt.Errorf("encoded updates too short")
	}
	if binary.LittleEndian.Uint32(raw[:4]) == gzipPackedConstructor {
		gz := &mtproto.TLGzipPacked{}
		if err := gz.Decode(mtproto.NewDecodeBuf(raw[4:])); err != nil {
			return nil, err
		}
		raw = gz.PackedData
	}
	if len(raw) < 4 {
		return nil, fmt.Errorf("encoded updates too short")
	}
	if binary.LittleEndian.Uint32(raw[:4]) != updatesConstructor {
		return nil, fmt.Errorf("encoded updates constructor 0x%x, want updates#74ae4240", binary.LittleEndian.Uint32(raw[:4]))
	}
	got := &mtproto.Updates{}
	if err := got.Decode(mtproto.NewDecodeBuf(raw)); err != nil {
		return nil, err
	}
	return got, nil
}
