package book

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// Passport scope is the JSON object from the authorization request.
// https://core.telegram.org/passport#passport-scope
// https://core.telegram.org/api/passport#uri-passport-scope
//
// Both the Bot API shape ({"data":[{"type":"passport"}],"v":1}) and the
// tg:// passport shape ({"d":["pp"],"v":1}) are accepted. v, when set, is 1.

const maxScopeLen = 8192

var typeAlias = map[string]string{
	"personal_details": mtproto.Predicate_secureValueTypePersonalDetails,
	"pd":               mtproto.Predicate_secureValueTypePersonalDetails,
	mtproto.Predicate_secureValueTypePersonalDetails: mtproto.Predicate_secureValueTypePersonalDetails,
	"passport": mtproto.Predicate_secureValueTypePassport,
	"pp":       mtproto.Predicate_secureValueTypePassport,
	mtproto.Predicate_secureValueTypePassport: mtproto.Predicate_secureValueTypePassport,
	"driver_license": mtproto.Predicate_secureValueTypeDriverLicense,
	"dl":             mtproto.Predicate_secureValueTypeDriverLicense,
	mtproto.Predicate_secureValueTypeDriverLicense: mtproto.Predicate_secureValueTypeDriverLicense,
	"identity_card": mtproto.Predicate_secureValueTypeIdentityCard,
	"ic":            mtproto.Predicate_secureValueTypeIdentityCard,
	mtproto.Predicate_secureValueTypeIdentityCard: mtproto.Predicate_secureValueTypeIdentityCard,
	"internal_passport":                           mtproto.Predicate_secureValueTypeInternalPassport,
	"ip":                                          mtproto.Predicate_secureValueTypeInternalPassport,
	mtproto.Predicate_secureValueTypeInternalPassport: mtproto.Predicate_secureValueTypeInternalPassport,
	"address":                                mtproto.Predicate_secureValueTypeAddress,
	"ad":                                     mtproto.Predicate_secureValueTypeAddress,
	mtproto.Predicate_secureValueTypeAddress: mtproto.Predicate_secureValueTypeAddress,
	"utility_bill":                           mtproto.Predicate_secureValueTypeUtilityBill,
	"ub":                                     mtproto.Predicate_secureValueTypeUtilityBill,
	mtproto.Predicate_secureValueTypeUtilityBill: mtproto.Predicate_secureValueTypeUtilityBill,
	"bank_statement": mtproto.Predicate_secureValueTypeBankStatement,
	"bs":             mtproto.Predicate_secureValueTypeBankStatement,
	mtproto.Predicate_secureValueTypeBankStatement: mtproto.Predicate_secureValueTypeBankStatement,
	"rental_agreement": mtproto.Predicate_secureValueTypeRentalAgreement,
	"ra":               mtproto.Predicate_secureValueTypeRentalAgreement,
	mtproto.Predicate_secureValueTypeRentalAgreement: mtproto.Predicate_secureValueTypeRentalAgreement,
	"passport_registration":                          mtproto.Predicate_secureValueTypePassportRegistration,
	"pr":                                             mtproto.Predicate_secureValueTypePassportRegistration,
	mtproto.Predicate_secureValueTypePassportRegistration: mtproto.Predicate_secureValueTypePassportRegistration,
	"temporary_registration":                              mtproto.Predicate_secureValueTypeTemporaryRegistration,
	"tr":                                                  mtproto.Predicate_secureValueTypeTemporaryRegistration,
	mtproto.Predicate_secureValueTypeTemporaryRegistration: mtproto.Predicate_secureValueTypeTemporaryRegistration,
	"phone_number":                         mtproto.Predicate_secureValueTypePhone,
	"phone":                                mtproto.Predicate_secureValueTypePhone,
	"pn":                                   mtproto.Predicate_secureValueTypePhone,
	mtproto.Predicate_secureValueTypePhone: mtproto.Predicate_secureValueTypePhone,
	"email":                                mtproto.Predicate_secureValueTypeEmail,
	"em":                                   mtproto.Predicate_secureValueTypeEmail,
	mtproto.Predicate_secureValueTypeEmail: mtproto.Predicate_secureValueTypeEmail,
}

type reqFlags struct {
	selfie      bool
	translation bool
	native      bool
}

func parseScope(raw string) ([]*mtproto.SecureRequiredType, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxScopeLen {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if v, ok := doc["v"]; ok {
		var n int
		if json.Unmarshal(v, &n) != nil || n != 1 {
			return nil, mtproto.ErrInputRequestInvalid
		}
	}
	elems, ok := doc["d"]
	if !ok {
		elems, ok = doc["data"]
	}
	if !ok {
		return nil, mtproto.ErrInputRequestInvalid
	}
	var list []json.RawMessage
	if json.Unmarshal(elems, &list) != nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	seen := map[string]bool{}
	out := make([]*mtproto.SecureRequiredType, 0, len(list))
	for _, el := range list {
		req, err := parseElement(el, reqFlags{})
		if err != nil {
			return nil, err
		}
		if err = markSeen(seen, req); err != nil {
			return nil, err
		}
		out = append(out, req)
	}
	return out, nil
}

func markSeen(seen map[string]bool, req *mtproto.SecureRequiredType) error {
	if req.GetPredicateName() == mtproto.Predicate_secureRequiredTypeOneOf {
		for _, inner := range req.GetTypes() {
			if err := markSeen(seen, inner); err != nil {
				return err
			}
		}
		return nil
	}
	key, err := typeKey(req.GetType())
	if err != nil {
		return mtproto.ErrInputRequestInvalid
	}
	if seen[key] {
		return mtproto.ErrInputRequestInvalid
	}
	seen[key] = true
	return nil
}

func parseElement(raw json.RawMessage, inherited reqFlags) (*mtproto.SecureRequiredType, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	if raw[0] == '"' {
		var name string
		if json.Unmarshal(raw, &name) != nil {
			return nil, mtproto.ErrInputRequestInvalid
		}
		return singleType(name, inherited)
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(raw, &obj) != nil {
		return nil, mtproto.ErrInputRequestInvalid
	}
	flags := mergeFlags(inherited, flagsOf(obj))
	if arr, ok := obj["one_of"]; ok {
		return parseOneOf(arr, flags)
	}
	if arr, ok := obj["_"]; ok && len(bytes.TrimSpace(arr)) > 0 && bytes.TrimSpace(arr)[0] == '[' {
		return parseOneOf(arr, flags)
	}
	name := ""
	if v, ok := obj["type"]; ok {
		_ = json.Unmarshal(v, &name)
	} else if v, ok := obj["_"]; ok {
		_ = json.Unmarshal(v, &name)
	}
	if name == "" {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return singleType(name, flags)
}

func parseOneOf(raw json.RawMessage, flags reqFlags) (*mtproto.SecureRequiredType, error) {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil || len(list) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	types := make([]*mtproto.SecureRequiredType, 0, len(list))
	for _, el := range list {
		req, err := parseElement(el, flags)
		if err != nil {
			return nil, err
		}
		if req.GetPredicateName() == mtproto.Predicate_secureRequiredTypeOneOf {
			types = append(types, req.GetTypes()...)
			continue
		}
		types = append(types, req)
	}
	if len(types) == 0 {
		return nil, mtproto.ErrInputRequestInvalid
	}
	return mtproto.MakeTLSecureRequiredTypeOneOf(&mtproto.SecureRequiredType{
		Types: types,
	}).To_SecureRequiredType(), nil
}

func singleType(name string, flags reqFlags) (*mtproto.SecureRequiredType, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "idd", "id_document":
		return parseOneOf(json.RawMessage(`["passport","driver_license","identity_card"]`), flags)
	case "add", "address_document":
		return parseOneOf(json.RawMessage(`["utility_bill","bank_statement","rental_agreement"]`), flags)
	}
	pred, ok := typeAlias[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		pred, ok = typeAlias[name]
	}
	if !ok {
		return nil, mtproto.ErrInputRequestInvalid
	}
	flags = allowedFlags(pred, flags)
	return mtproto.MakeTLSecureRequiredType(&mtproto.SecureRequiredType{
		NativeNames:         flags.native,
		SelfieRequired:      flags.selfie,
		TranslationRequired: flags.translation,
		Type:                secureType(pred),
	}).To_SecureRequiredType(), nil
}

func allowedFlags(pred string, flags reqFlags) reqFlags {
	switch pred {
	case mtproto.Predicate_secureValueTypePersonalDetails:
		flags.selfie = false
		flags.translation = false
	case mtproto.Predicate_secureValueTypePassport,
		mtproto.Predicate_secureValueTypeDriverLicense,
		mtproto.Predicate_secureValueTypeIdentityCard,
		mtproto.Predicate_secureValueTypeInternalPassport:
		flags.native = false
	case mtproto.Predicate_secureValueTypeUtilityBill,
		mtproto.Predicate_secureValueTypeBankStatement,
		mtproto.Predicate_secureValueTypeRentalAgreement,
		mtproto.Predicate_secureValueTypePassportRegistration,
		mtproto.Predicate_secureValueTypeTemporaryRegistration:
		flags.native = false
		flags.selfie = false
	default:
		flags = reqFlags{}
	}
	return flags
}

func flagsOf(obj map[string]json.RawMessage) reqFlags {
	return reqFlags{
		selfie:      jsonFlag(obj, "s", "selfie"),
		translation: jsonFlag(obj, "t", "translation"),
		native:      jsonFlag(obj, "n", "native_names"),
	}
}

func mergeFlags(base, extra reqFlags) reqFlags {
	if extra.selfie {
		base.selfie = true
	}
	if extra.translation {
		base.translation = true
	}
	if extra.native {
		base.native = true
	}
	return base
}

func jsonFlag(obj map[string]json.RawMessage, keys ...string) bool {
	for _, key := range keys {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		s := strings.Trim(string(bytes.TrimSpace(raw)), `"`)
		if s == "true" || s == "1" {
			return true
		}
	}
	return false
}

func scopeTypes(required []*mtproto.SecureRequiredType) map[string]bool {
	out := map[string]bool{}
	var walk func(*mtproto.SecureRequiredType)
	walk = func(req *mtproto.SecureRequiredType) {
		if req == nil {
			return
		}
		if req.GetPredicateName() == mtproto.Predicate_secureRequiredTypeOneOf {
			for _, inner := range req.GetTypes() {
				walk(inner)
			}
			return
		}
		if key, err := typeKey(req.GetType()); err == nil {
			out[key] = true
		}
	}
	for _, req := range required {
		walk(req)
	}
	return out
}
