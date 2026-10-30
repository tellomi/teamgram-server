package dao

import (
	"testing"

	"github.com/teamgram/proto/mtproto"
)

func TestLoginColumnsForNewUser(t *testing.T) {
	phone, err := loginColumnsForNewUser("8613800138000", "CN")
	if err != nil {
		t.Fatal(err)
	}
	if phone.EmailOnly || phone.Phone != "8613800138000" || phone.CountryCode != "CN" || phone.Email.Valid {
		t.Fatalf("%+v", phone)
	}

	email, err := loginColumnsForNewUser(" User@Tellomi.app ", "CN")
	if err != nil {
		t.Fatal(err)
	}
	if !email.EmailOnly || email.Phone != "" || email.CountryCode != "" || !email.Email.Valid || email.Email.String != "user@tellomi.app" {
		t.Fatalf("%+v", email)
	}

	if _, err := loginColumnsForNewUser("a@b", ""); err != mtproto.ErrEmailInvalid {
		t.Fatal(err)
	}
}
