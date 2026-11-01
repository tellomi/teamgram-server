package book

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"strings"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/phonenumber"
	"google.golang.org/protobuf/proto"
)

// Field rules follow https://core.telegram.org/api/passport#securevalue.
// Hashes are SHA-256 (32 bytes). A value with one part uses that part's hash,
// which is what account.acceptAuthorization calls the data_hash or file_hash.
// Several parts are mixed with SHA-256 over the concatenation, in the order
// data, front, reverse, selfie, translation, files, then the plain phone or
// email. The server returns that hash; the client sends it back.

const (
	hashLen       = 32
	maxSecureData = 256 << 10
	maxFiles      = 20
	maxParts      = 8000
	dcID          = int32(1)
)

type policy struct {
	data        bool
	front       bool
	reverse     bool
	files       bool
	selfie      bool
	translation bool
	plain       string
}

var policies = map[string]policy{
	mtproto.Predicate_secureValueTypePersonalDetails:       {data: true},
	mtproto.Predicate_secureValueTypePassport:              {data: true, front: true, selfie: true, translation: true},
	mtproto.Predicate_secureValueTypeInternalPassport:      {data: true, front: true, selfie: true, translation: true},
	mtproto.Predicate_secureValueTypeDriverLicense:         {data: true, front: true, reverse: true, selfie: true, translation: true},
	mtproto.Predicate_secureValueTypeIdentityCard:          {data: true, front: true, reverse: true, selfie: true, translation: true},
	mtproto.Predicate_secureValueTypeAddress:               {data: true},
	mtproto.Predicate_secureValueTypeUtilityBill:           {files: true, translation: true},
	mtproto.Predicate_secureValueTypeBankStatement:         {files: true, translation: true},
	mtproto.Predicate_secureValueTypeRentalAgreement:       {files: true, translation: true},
	mtproto.Predicate_secureValueTypePassportRegistration:  {files: true, translation: true},
	mtproto.Predicate_secureValueTypeTemporaryRegistration: {files: true, translation: true},
	mtproto.Predicate_secureValueTypePhone:                 {plain: "phone"},
	mtproto.Predicate_secureValueTypeEmail:                 {plain: "email"},
}

var typeOrder = []string{
	mtproto.Predicate_secureValueTypePersonalDetails,
	mtproto.Predicate_secureValueTypePassport,
	mtproto.Predicate_secureValueTypeDriverLicense,
	mtproto.Predicate_secureValueTypeIdentityCard,
	mtproto.Predicate_secureValueTypeInternalPassport,
	mtproto.Predicate_secureValueTypeAddress,
	mtproto.Predicate_secureValueTypeUtilityBill,
	mtproto.Predicate_secureValueTypeBankStatement,
	mtproto.Predicate_secureValueTypeRentalAgreement,
	mtproto.Predicate_secureValueTypePassportRegistration,
	mtproto.Predicate_secureValueTypeTemporaryRegistration,
	mtproto.Predicate_secureValueTypePhone,
	mtproto.Predicate_secureValueTypeEmail,
}

func typeKey(t *mtproto.SecureValueType) (string, error) {
	if t == nil {
		return "", mtproto.ErrDataInvalid
	}
	name := t.GetPredicateName()
	if name == "" {
		name = predicateByConstructor(t.GetConstructor())
	}
	if _, ok := policies[name]; !ok {
		return "", mtproto.ErrDataInvalid
	}
	return name, nil
}

func predicateByConstructor(id mtproto.TLConstructor) string {
	switch id {
	case mtproto.CRC32_secureValueTypePersonalDetails:
		return mtproto.Predicate_secureValueTypePersonalDetails
	case mtproto.CRC32_secureValueTypePassport:
		return mtproto.Predicate_secureValueTypePassport
	case mtproto.CRC32_secureValueTypeDriverLicense:
		return mtproto.Predicate_secureValueTypeDriverLicense
	case mtproto.CRC32_secureValueTypeIdentityCard:
		return mtproto.Predicate_secureValueTypeIdentityCard
	case mtproto.CRC32_secureValueTypeInternalPassport:
		return mtproto.Predicate_secureValueTypeInternalPassport
	case mtproto.CRC32_secureValueTypeAddress:
		return mtproto.Predicate_secureValueTypeAddress
	case mtproto.CRC32_secureValueTypeUtilityBill:
		return mtproto.Predicate_secureValueTypeUtilityBill
	case mtproto.CRC32_secureValueTypeBankStatement:
		return mtproto.Predicate_secureValueTypeBankStatement
	case mtproto.CRC32_secureValueTypeRentalAgreement:
		return mtproto.Predicate_secureValueTypeRentalAgreement
	case mtproto.CRC32_secureValueTypePassportRegistration:
		return mtproto.Predicate_secureValueTypePassportRegistration
	case mtproto.CRC32_secureValueTypeTemporaryRegistration:
		return mtproto.Predicate_secureValueTypeTemporaryRegistration
	case mtproto.CRC32_secureValueTypePhone:
		return mtproto.Predicate_secureValueTypePhone
	case mtproto.CRC32_secureValueTypeEmail:
		return mtproto.Predicate_secureValueTypeEmail
	default:
		return ""
	}
}

func secureType(pred string) *mtproto.SecureValueType {
	switch pred {
	case mtproto.Predicate_secureValueTypePersonalDetails:
		return mtproto.MakeTLSecureValueTypePersonalDetails(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypePassport:
		return mtproto.MakeTLSecureValueTypePassport(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypeDriverLicense:
		return mtproto.MakeTLSecureValueTypeDriverLicense(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypeIdentityCard:
		return mtproto.MakeTLSecureValueTypeIdentityCard(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypeInternalPassport:
		return mtproto.MakeTLSecureValueTypeInternalPassport(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypeAddress:
		return mtproto.MakeTLSecureValueTypeAddress(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypeUtilityBill:
		return mtproto.MakeTLSecureValueTypeUtilityBill(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypeBankStatement:
		return mtproto.MakeTLSecureValueTypeBankStatement(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypeRentalAgreement:
		return mtproto.MakeTLSecureValueTypeRentalAgreement(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypePassportRegistration:
		return mtproto.MakeTLSecureValueTypePassportRegistration(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypeTemporaryRegistration:
		return mtproto.MakeTLSecureValueTypeTemporaryRegistration(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypePhone:
		return mtproto.MakeTLSecureValueTypePhone(nil).To_SecureValueType()
	case mtproto.Predicate_secureValueTypeEmail:
		return mtproto.MakeTLSecureValueTypeEmail(nil).To_SecureValueType()
	default:
		return nil
	}
}

func (u *userBox) convert(in *mtproto.InputSecureValue, now int32) (*mtproto.SecureValue, error) {
	if in == nil {
		return nil, mtproto.ErrDataInvalid
	}
	key, err := typeKey(in.GetType())
	if err != nil {
		return nil, err
	}
	rule := policies[key]
	if !rule.data && in.GetData() != nil {
		return nil, mtproto.ErrDataInvalid
	}
	if !rule.front && in.GetFrontSide() != nil {
		return nil, mtproto.ErrDataInvalid
	}
	if !rule.reverse && in.GetReverseSide() != nil {
		return nil, mtproto.ErrDataInvalid
	}
	if !rule.selfie && in.GetSelfie() != nil {
		return nil, mtproto.ErrDataInvalid
	}
	if !rule.translation && len(in.GetTranslation()) > 0 {
		return nil, mtproto.ErrDataInvalid
	}
	if !rule.files && len(in.GetFiles()) > 0 {
		return nil, mtproto.ErrDataInvalid
	}
	if rule.plain == "" && in.GetPlainData() != nil {
		return nil, mtproto.ErrDataInvalid
	}
	if len(in.GetTranslation()) > maxFiles || len(in.GetFiles()) > maxFiles {
		return nil, mtproto.ErrDataInvalid
	}

	out := &mtproto.SecureValue{Type: secureType(key)}
	var parts [][]byte
	if rule.data {
		data, err := copyData(in.GetData())
		if err != nil {
			return nil, err
		}
		out.Data = data
		parts = append(parts, data.GetDataHash())
	}
	take := func(file *mtproto.InputSecureFile, required bool) (*mtproto.SecureFile, error) {
		if file == nil {
			if required {
				return nil, mtproto.ErrDataInvalid
			}
			return nil, nil
		}
		return u.secureFile(file, now)
	}
	if out.FrontSide, err = take(in.GetFrontSide(), rule.front); err != nil {
		return nil, err
	}
	if out.FrontSide != nil {
		parts = append(parts, out.FrontSide.GetFileHash())
	}
	if out.ReverseSide, err = take(in.GetReverseSide(), rule.reverse); err != nil {
		return nil, err
	}
	if out.ReverseSide != nil {
		parts = append(parts, out.ReverseSide.GetFileHash())
	}
	if out.Selfie, err = take(in.GetSelfie(), false); err != nil {
		return nil, err
	}
	if out.Selfie != nil {
		parts = append(parts, out.Selfie.GetFileHash())
	}
	for _, file := range in.GetTranslation() {
		got, err := u.secureFile(file, now)
		if err != nil {
			return nil, err
		}
		out.Translation = append(out.Translation, got)
		parts = append(parts, got.GetFileHash())
	}
	if rule.files && len(in.GetFiles()) == 0 {
		return nil, mtproto.ErrDataInvalid
	}
	for _, file := range in.GetFiles() {
		got, err := u.secureFile(file, now)
		if err != nil {
			return nil, err
		}
		out.Files = append(out.Files, got)
		parts = append(parts, got.GetFileHash())
	}
	switch rule.plain {
	case "phone":
		plain, digest, err := copyPhone(in.GetPlainData(), u.verified)
		if err != nil {
			return nil, err
		}
		out.PlainData = plain
		parts = append(parts, digest)
	case "email":
		plain, digest, err := copyEmail(in.GetPlainData())
		if err != nil {
			return nil, err
		}
		out.PlainData = plain
		parts = append(parts, digest)
	}
	if len(parts) == 0 {
		return nil, mtproto.ErrDataInvalid
	}
	out.Hash = valueHash(parts)
	return mtproto.MakeTLSecureValue(out).To_SecureValue(), nil
}

func valueHash(parts [][]byte) []byte {
	if len(parts) == 1 {
		return append([]byte(nil), parts[0]...)
	}
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write(part)
	}
	return h.Sum(nil)
}

func copyData(in *mtproto.SecureData) (*mtproto.SecureData, error) {
	if in == nil || len(in.GetData()) == 0 || len(in.GetDataHash()) == 0 || len(in.GetSecret()) == 0 {
		return nil, mtproto.ErrDataInvalid
	}
	if len(in.GetData()) > maxSecureData {
		return nil, mtproto.ErrDataInvalid
	}
	if len(in.GetDataHash()) != hashLen {
		return nil, mtproto.ErrDataHashSizeInvalid
	}
	if len(in.GetSecret()) != hashLen {
		return nil, mtproto.ErrDataInvalid
	}
	return mtproto.MakeTLSecureData(&mtproto.SecureData{
		Data:     append([]byte(nil), in.GetData()...),
		DataHash: append([]byte(nil), in.GetDataHash()...),
		Secret:   append([]byte(nil), in.GetSecret()...),
	}).To_SecureData(), nil
}

func (u *userBox) secureFile(in *mtproto.InputSecureFile, now int32) (*mtproto.SecureFile, error) {
	if in == nil || in.GetId() == 0 {
		return nil, mtproto.ErrFileIdInvalid
	}
	name := in.GetPredicateName()
	if name == "" {
		if in.GetAccessHash() != 0 && len(in.GetFileHash()) == 0 {
			name = mtproto.Predicate_inputSecureFile
		} else {
			name = mtproto.Predicate_inputSecureFileUploaded
		}
	}
	switch name {
	case mtproto.Predicate_inputSecureFile:
		prev := u.files[in.GetId()]
		if prev == nil || prev.GetAccessHash() == 0 || prev.GetAccessHash() != in.GetAccessHash() {
			return nil, mtproto.ErrFileIdInvalid
		}
		return cloneFile(prev), nil
	case mtproto.Predicate_inputSecureFileUploaded:
		if in.GetParts() < 1 || in.GetParts() > maxParts {
			return nil, mtproto.ErrFilePartsInvalid
		}
		if len(in.GetFileHash()) != hashLen {
			return nil, mtproto.ErrHashSizeInvalid
		}
		if len(in.GetSecret()) != hashLen {
			return nil, mtproto.ErrDataInvalid
		}
		return mtproto.MakeTLSecureFile(&mtproto.SecureFile{
			Id:         in.GetId(),
			AccessHash: randInt63(),
			DcId:       dcID,
			Date:       now,
			FileHash:   append([]byte(nil), in.GetFileHash()...),
			Secret:     append([]byte(nil), in.GetSecret()...),
		}).To_SecureFile(), nil
	default:
		return nil, mtproto.ErrFileIdInvalid
	}
}

func copyPhone(in *mtproto.SecurePlainData, verified map[string]struct{}) (*mtproto.SecurePlainData, []byte, error) {
	if in == nil {
		return nil, nil, mtproto.ErrDataInvalid
	}
	name := in.GetPredicateName()
	if name != "" && name != mtproto.Predicate_securePlainPhone {
		return nil, nil, mtproto.ErrDataInvalid
	}
	phone := strings.TrimSpace(in.GetPhone())
	norm, err := phonenumber.CheckAndGetPhoneNumber(phone)
	if err != nil {
		return nil, nil, mtproto.ErrPhoneNumberInvalid
	}
	if _, ok := verified[norm]; !ok {
		return nil, nil, mtproto.ErrPhoneNumberInvalid
	}
	sum := sha256.Sum256([]byte(phone))
	return mtproto.MakeTLSecurePlainPhone(&mtproto.SecurePlainData{Phone: phone}).To_SecurePlainData(), sum[:], nil
}

func copyEmail(in *mtproto.SecurePlainData) (*mtproto.SecurePlainData, []byte, error) {
	if in == nil {
		return nil, nil, mtproto.ErrDataInvalid
	}
	name := in.GetPredicateName()
	if name != "" && name != mtproto.Predicate_securePlainEmail {
		return nil, nil, mtproto.ErrDataInvalid
	}
	email := strings.TrimSpace(in.GetEmail())
	if !validEmail(email) {
		return nil, nil, mtproto.ErrEmailInvalid
	}
	sum := sha256.Sum256([]byte(email))
	return mtproto.MakeTLSecurePlainEmail(&mtproto.SecurePlainData{Email: email}).To_SecurePlainData(), sum[:], nil
}

func validEmail(s string) bool {
	if len(s) < 3 || len(s) > 128 || strings.ContainsAny(s, " \t\r\n") {
		return false
	}
	at := strings.IndexByte(s, '@')
	if at <= 0 || at != strings.LastIndexByte(s, '@') {
		return false
	}
	host := s[at+1:]
	return strings.Contains(host, ".") && !strings.HasPrefix(host, ".") && !strings.HasSuffix(host, ".")
}

func componentHashes(v *mtproto.SecureValue) map[string]struct{} {
	out := map[string]struct{}{}
	add := func(b []byte) {
		if len(b) > 0 {
			out[string(b)] = struct{}{}
		}
	}
	if v == nil {
		return out
	}
	add(v.GetHash())
	if v.GetData() != nil {
		add(v.GetData().GetDataHash())
	}
	addFile := func(f *mtproto.SecureFile) { add(f.GetFileHash()) }
	addFile(v.GetFrontSide())
	addFile(v.GetReverseSide())
	addFile(v.GetSelfie())
	for _, f := range v.GetTranslation() {
		addFile(f)
	}
	for _, f := range v.GetFiles() {
		addFile(f)
	}
	if plain := v.GetPlainData(); plain != nil {
		switch plain.GetPredicateName() {
		case mtproto.Predicate_securePlainPhone:
			sum := sha256.Sum256([]byte(plain.GetPhone()))
			add(sum[:])
		case mtproto.Predicate_securePlainEmail:
			sum := sha256.Sum256([]byte(plain.GetEmail()))
			add(sum[:])
		}
	}
	return out
}

func randInt63() int64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 1
	}
	n := int64(binary.BigEndian.Uint64(buf[:]) & 0x7fffffffffffffff)
	if n == 0 {
		return 1
	}
	return n
}

func cloneValue(v *mtproto.SecureValue) *mtproto.SecureValue {
	if v == nil {
		return nil
	}
	return proto.Clone(v).(*mtproto.SecureValue)
}

func cloneFile(v *mtproto.SecureFile) *mtproto.SecureFile {
	if v == nil {
		return nil
	}
	return proto.Clone(v).(*mtproto.SecureFile)
}

func cloneError(v *mtproto.SecureValueError) *mtproto.SecureValueError {
	if v == nil {
		return nil
	}
	return proto.Clone(v).(*mtproto.SecureValueError)
}

func sameHash(a, b []byte) bool {
	return len(a) > 0 && bytes.Equal(a, b)
}
