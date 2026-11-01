package book

import (
	"bytes"
	"context"
	"crypto/sha256"
	"testing"
	"time"

	"github.com/teamgram/proto/mtproto"
)

func TestConfigHashStable(t *testing.T) {
	if ConfigHash() == 0 {
		t.Fatal("hash")
	}
	if !bytes.Contains([]byte(CountriesLangs()), []byte(`"CN":"zh"`)) {
		t.Fatal(CountriesLangs())
	}
	if ConfigHash() != ConfigHash() {
		t.Fatal("unstable")
	}
}

func TestSaveGetDeleteAndSecret(t *testing.T) {
	ctx := context.Background()
	b := New(nil)
	if _, err := b.Save(ctx, 7, 0, personal("cipher")); err != mtproto.ErrSecureSecretRequired {
		t.Fatal(err)
	}
	saved, err := b.Save(ctx, 7, 11, personal("cipher"))
	if err != nil {
		t.Fatal(err)
	}
	if len(saved.GetHash()) != 32 || !bytes.Equal(saved.GetHash(), saved.GetData().GetDataHash()) {
		t.Fatalf("hash %#v", saved.GetHash())
	}
	all, err := b.All(ctx, 7)
	if err != nil || len(all) != 1 {
		t.Fatalf("%v %d", err, len(all))
	}
	got, err := b.Get(ctx, 7, []*mtproto.SecureValueType{secureType(mtproto.Predicate_secureValueTypePersonalDetails)})
	if err != nil || len(got) != 1 || got[0].GetType().GetPredicateName() != mtproto.Predicate_secureValueTypePersonalDetails {
		t.Fatal(err, got)
	}
	delete(b.users, 7)
	reloaded, err := b.All(ctx, 7)
	if err != nil || len(reloaded) != 1 {
		t.Fatalf("reload %v %d", err, len(reloaded))
	}
	bad := personal("other")
	bad.Data = nil
	if _, err = b.Save(ctx, 7, 12, bad); err != mtproto.ErrDataInvalid {
		t.Fatalf("rejected rotation: %v", err)
	}
	all, _ = b.All(ctx, 7)
	if len(all) != 1 || string(all[0].GetData().GetData()) != "cipher" {
		t.Fatalf("failed rotation wiped %+v", all)
	}
	if _, err = b.Save(ctx, 7, 12, personal("other")); err != nil {
		t.Fatal(err)
	}
	all, _ = b.All(ctx, 7)
	if len(all) != 1 || string(all[0].GetData().GetData()) != "other" {
		t.Fatalf("rotation %+v", all)
	}
	if err = b.Delete(ctx, 7, []*mtproto.SecureValueType{secureType(mtproto.Predicate_secureValueTypeEmail)}); err != nil {
		t.Fatal(err)
	}
	all, _ = b.All(ctx, 7)
	if len(all) != 1 {
		t.Fatalf("unrelated delete %d", len(all))
	}
	if err = b.Delete(ctx, 7, []*mtproto.SecureValueType{all[0].GetType()}); err != nil {
		t.Fatal(err)
	}
	all, _ = b.All(ctx, 7)
	if len(all) != 0 {
		t.Fatalf("deleted %d", len(all))
	}
}

func TestDocumentShape(t *testing.T) {
	ctx := context.Background()
	b := New(nil)
	pass := mtproto.MakeTLInputSecureValue(&mtproto.InputSecureValue{
		Type: secureType(mtproto.Predicate_secureValueTypePassport),
		Data: dataBlob("doc"),
	}).To_InputSecureValue()
	if _, err := b.Save(ctx, 3, 1, pass); err != mtproto.ErrDataInvalid {
		t.Fatalf("front required: %v", err)
	}
	pass.FrontSide = uploaded(9)
	saved, err := b.Save(ctx, 3, 1, pass)
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetFrontSide().GetId() != 9 || saved.GetFrontSide().GetAccessHash() == 0 || saved.GetFrontSide().GetDcId() != 1 {
		t.Fatalf("file %+v", saved.GetFrontSide())
	}
	again := mtproto.MakeTLInputSecureValue(&mtproto.InputSecureValue{
		Type: secureType(mtproto.Predicate_secureValueTypePassport),
		Data: dataBlob("doc"),
		FrontSide: mtproto.MakeTLInputSecureFile(&mtproto.InputSecureFile{
			Id:         saved.GetFrontSide().GetId(),
			AccessHash: saved.GetFrontSide().GetAccessHash(),
		}).To_InputSecureFile(),
	}).To_InputSecureValue()
	if _, err = b.Save(ctx, 3, 1, again); err != nil {
		t.Fatal(err)
	}
	bad := uploaded(8)
	bad.FileHash = []byte{1, 2, 3}
	pass.FrontSide = bad
	if _, err = b.Save(ctx, 3, 1, pass); err != mtproto.ErrHashSizeInvalid {
		t.Fatalf("hash size: %v", err)
	}
}

func TestPhoneAndEmail(t *testing.T) {
	ctx := context.Background()
	b := New(nil)
	b.at(time.Unix(1_700_000_000, 0))
	phone := "9996612345"
	hash, code, err := b.SendPhoneCode(ctx, 4, phone)
	if err != nil {
		t.Fatal(err)
	}
	if code != "11111" || hash == "" {
		t.Fatalf("code %s hash %s", code, hash)
	}
	if err = b.VerifyPhone(ctx, 4, phone, hash, "00000"); err != mtproto.ErrPhoneCodeInvalid {
		t.Fatal(err)
	}
	if err = b.VerifyPhone(ctx, 4, phone, hash, code); err != nil {
		t.Fatal(err)
	}
	if err = b.VerifyPhone(ctx, 4, phone, hash, code); err != mtproto.ErrPhoneCodeExpired {
		t.Fatal(err)
	}
	saved, err := b.Save(ctx, 4, 5, mtproto.MakeTLInputSecureValue(&mtproto.InputSecureValue{
		Type:      secureType(mtproto.Predicate_secureValueTypePhone),
		PlainData: mtproto.MakeTLSecurePlainPhone(&mtproto.SecurePlainData{Phone: phone}).To_SecurePlainData(),
	}).To_InputSecureValue())
	if err != nil {
		t.Fatal(err)
	}
	if saved.GetPlainData().GetPhone() != phone {
		t.Fatal(saved.GetPlainData())
	}
	if _, err = b.Save(ctx, 4, 5, mtproto.MakeTLInputSecureValue(&mtproto.InputSecureValue{
		Type:      secureType(mtproto.Predicate_secureValueTypeEmail),
		PlainData: mtproto.MakeTLSecurePlainEmail(&mtproto.SecurePlainData{Email: "not-an-email"}).To_SecurePlainData(),
	}).To_InputSecureValue()); err != mtproto.ErrEmailInvalid {
		t.Fatal(err)
	}
	b.at(time.Unix(1_700_000_000, 0).Add(codeTTL + time.Second))
	hash, _, err = b.SendPhoneCode(ctx, 4, phone)
	if err != nil {
		t.Fatal(err)
	}
	b.at(time.Unix(1_700_000_000, 0).Add(2*codeTTL + time.Second))
	if err = b.VerifyPhone(ctx, 4, phone, hash, "11111"); err != mtproto.ErrPhoneCodeExpired {
		t.Fatal(err)
	}
}

func TestScopeFormAcceptAndErrors(t *testing.T) {
	ctx := context.Background()
	b := New(nil)
	const (
		user  = int64(8)
		bot   = int64(90)
		scope = `{"v":1,"d":[{"_":"pd","n":1},"pn",{"_":["pp","dl"],"s":1}]}`
		key   = "-----BEGIN PUBLIC KEY-----\nMIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA\n-----END PUBLIC KEY-----"
	)
	if _, err := b.AuthorizationForm(ctx, user, bot, scope, ""); err != mtproto.ErrPublicKeyRequired {
		t.Fatal(err)
	}
	if _, err := b.AuthorizationForm(ctx, user, bot, "{", key); err != mtproto.ErrInputRequestInvalid {
		t.Fatal(err)
	}
	form, err := b.AuthorizationForm(ctx, user, bot, scope, key)
	if err != nil {
		t.Fatal(err)
	}
	if len(form.Required) != 3 || !form.Required[0].GetNativeNames() {
		t.Fatalf("required %+v", form.Required)
	}
	one := form.Required[2]
	if one.GetPredicateName() != mtproto.Predicate_secureRequiredTypeOneOf || len(one.GetTypes()) != 2 || !one.GetTypes()[0].GetSelfieRequired() {
		t.Fatalf("one_of %+v", one)
	}
	saved, err := b.Save(ctx, user, 3, personal("ada"))
	if err != nil {
		t.Fatal(err)
	}
	badHash := bytes.Repeat([]byte{7}, 31)
	if err = b.SetErrors(ctx, user, bot, []*mtproto.SecureValueError{
		mtproto.MakeTLSecureValueErrorData(&mtproto.SecureValueError{
			Type:     secureType(mtproto.Predicate_secureValueTypePersonalDetails),
			DataHash: badHash,
			Field:    "first_name",
			Text:     "blurry",
		}).To_SecureValueError(),
	}); err != mtproto.ErrDataHashSizeInvalid {
		t.Fatal(err)
	}
	if err = b.SetErrors(ctx, user, bot, []*mtproto.SecureValueError{
		mtproto.MakeTLSecureValueErrorData(&mtproto.SecureValueError{
			Type:     secureType(mtproto.Predicate_secureValueTypePersonalDetails),
			DataHash: saved.GetData().GetDataHash(),
			Field:    "first_name",
			Text:     "blurry",
		}).To_SecureValueError(),
	}); err != nil {
		t.Fatal(err)
	}
	form, err = b.AuthorizationForm(ctx, user, bot, scope, key)
	if err != nil || len(form.Errors) != 1 || len(form.Values) != 1 {
		t.Fatalf("form %v values %d errors %d", err, len(form.Values), len(form.Errors))
	}
	payload := []byte("creds")
	sum := sha256.Sum256(payload)
	creds := &mtproto.SecureCredentialsEncrypted{Data: payload, Hash: sum[:], Secret: bytes.Repeat([]byte{4}, 32)}
	hashes := []*mtproto.SecureValueHash{mtproto.MakeTLSecureValueHash(&mtproto.SecureValueHash{
		Type: saved.GetType(),
		Hash: saved.GetHash(),
	}).To_SecureValueHash()}
	if err = b.Accept(ctx, user, bot, scope, key, hashes, creds); err != mtproto.ErrDataInvalid {
		t.Fatalf("unresolved and incomplete: %v", err)
	}
	if err = b.SetErrors(ctx, user, bot, nil); err != nil {
		t.Fatal(err)
	}
	phone := "9996612345"
	codeHash, code, err := b.SendPhoneCode(ctx, user, phone)
	if err != nil {
		t.Fatal(err)
	}
	if err = b.VerifyPhone(ctx, user, phone, codeHash, code); err != nil {
		t.Fatal(err)
	}
	phoneValue, err := b.Save(ctx, user, 3, mtproto.MakeTLInputSecureValue(&mtproto.InputSecureValue{
		Type:      secureType(mtproto.Predicate_secureValueTypePhone),
		PlainData: mtproto.MakeTLSecurePlainPhone(&mtproto.SecurePlainData{Phone: phone}).To_SecurePlainData(),
	}).To_InputSecureValue())
	if err != nil {
		t.Fatal(err)
	}
	pass, err := b.Save(ctx, user, 3, mtproto.MakeTLInputSecureValue(&mtproto.InputSecureValue{
		Type:      secureType(mtproto.Predicate_secureValueTypePassport),
		Data:      dataBlob("pass"),
		FrontSide: uploaded(4),
		Selfie:    uploaded(5),
	}).To_InputSecureValue())
	if err != nil {
		t.Fatal(err)
	}
	hashes = []*mtproto.SecureValueHash{
		hashOf(saved), hashOf(phoneValue), hashOf(pass),
	}
	if err = b.Accept(ctx, user, bot, scope, key, hashes, &mtproto.SecureCredentialsEncrypted{
		Data: payload, Hash: []byte("nope"), Secret: creds.Secret,
	}); err != mtproto.ErrDataInvalid {
		t.Fatal(err)
	}
	if err = b.Accept(ctx, user, bot, scope, key, hashes, creds); err != nil {
		t.Fatal(err)
	}
	grants, err := b.Grants(ctx, user)
	if err != nil || len(grants) != 1 || grants[0].BotID != bot || len(grants[0].Types) != 3 {
		t.Fatalf("%v %+v", err, grants)
	}
	fresh, err := b.Save(ctx, user, 3, personal("zed-fixed"))
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(fresh.GetData().GetData(), saved.GetData().GetData()) {
		t.Fatal("value did not change")
	}
}

func personal(body string) *mtproto.InputSecureValue {
	return mtproto.MakeTLInputSecureValue(&mtproto.InputSecureValue{
		Type: secureType(mtproto.Predicate_secureValueTypePersonalDetails),
		Data: dataBlob(body),
	}).To_InputSecureValue()
}

func dataBlob(body string) *mtproto.SecureData {
	return mtproto.MakeTLSecureData(&mtproto.SecureData{
		Data:     []byte(body),
		DataHash: bytes.Repeat([]byte{body[0]}, 32),
		Secret:   bytes.Repeat([]byte{2}, 32),
	}).To_SecureData()
}

func uploaded(id int64) *mtproto.InputSecureFile {
	return mtproto.MakeTLInputSecureFileUploaded(&mtproto.InputSecureFile{
		Id:       id,
		Parts:    1,
		FileHash: bytes.Repeat([]byte{byte(id)}, 32),
		Secret:   bytes.Repeat([]byte{9}, 32),
	}).To_InputSecureFile()
}

func hashOf(v *mtproto.SecureValue) *mtproto.SecureValueHash {
	return mtproto.MakeTLSecureValueHash(&mtproto.SecureValueHash{
		Type: v.GetType(),
		Hash: v.GetHash(),
	}).To_SecureValueHash()
}
