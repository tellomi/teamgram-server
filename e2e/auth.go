package e2e

import (
	"context"
	"fmt"

	"github.com/teamgram/proto/mtproto"
)

// Self is the logged-in user.
type Self struct {
	ID         int64
	AccessHash int64
	Username   string
}

// Login runs auth.sendCode, auth.signIn, and auth.signUp when the number is new.
func (c *Client) Login(ctx context.Context, phone Phone, firstName string) (Self, error) {
	sent, err := c.Invoke(ctx, &mtproto.TLAuthSendCode{
		Constructor: mtproto.CRC32_auth_sendCode,
		PhoneNumber: phone.Number,
		ApiId:       c.cfg.APIID,
		ApiHash:     c.cfg.APIHash,
		Settings:    mtproto.MakeTLCodeSettings(&mtproto.CodeSettings{}).To_CodeSettings(),
	})
	if err != nil {
		return Self{}, fmt.Errorf("auth.sendCode: %w", err)
	}
	code, ok := sent.(*mtproto.TLAuthSentCode)
	if !ok {
		return Self{}, fmt.Errorf("auth.sendCode returned %T", sent)
	}
	hash := code.GetPhoneCodeHash()
	if hash == "" {
		return Self{}, fmt.Errorf("auth.sendCode returned an empty phone_code_hash")
	}

	signed, err := c.Invoke(ctx, &mtproto.TLAuthSignIn{
		Constructor:          mtproto.CRC32_auth_signIn_8d52a951,
		PhoneNumber:          phone.Number,
		PhoneCodeHash:        hash,
		PhoneCode_FLAGSTRING: strFlag(phone.Code),
	})
	if err != nil {
		return Self{}, fmt.Errorf("auth.signIn: %w", err)
	}
	switch authz := signed.(type) {
	case *mtproto.TLAuthAuthorization:
		return selfFrom(authz.GetUser())
	case *mtproto.TLAuthAuthorizationSignUpRequired:
		signed, err = c.Invoke(ctx, &mtproto.TLAuthSignUp{
			Constructor:   mtproto.CRC32_auth_signUp_aac7b717,
			PhoneNumber:   phone.Number,
			PhoneCodeHash: hash,
			FirstName:     firstName,
			LastName:      "",
		})
		if err != nil {
			return Self{}, fmt.Errorf("auth.signUp: %w", err)
		}
		created, ok := signed.(*mtproto.TLAuthAuthorization)
		if !ok {
			return Self{}, fmt.Errorf("auth.signUp returned %T", signed)
		}
		return selfFrom(created.GetUser())
	default:
		return Self{}, fmt.Errorf("auth.signIn returned %T", signed)
	}
}

func selfFrom(user *mtproto.User) (Self, error) {
	if user == nil || user.GetId() == 0 {
		return Self{}, fmt.Errorf("authorization has no user")
	}
	var hash int64
	if user.GetAccessHash() != nil {
		hash = user.GetAccessHash().GetValue()
	}
	name := ""
	if user.GetUsername() != nil {
		name = user.GetUsername().GetValue()
	}
	return Self{ID: user.GetId(), AccessHash: hash, Username: name}, nil
}

// ImportPhone resolves another test account and returns its input peer.
func (c *Client) ImportPhone(ctx context.Context, phone, firstName string) (id, accessHash int64, err error) {
	res, err := c.Invoke(ctx, &mtproto.TLContactsImportContacts{
		Constructor: mtproto.CRC32_contacts_importContacts,
		Contacts: []*mtproto.InputContact{
			mtproto.MakeTLInputPhoneContact(&mtproto.InputContact{
				ClientId:  1,
				Phone:     phone,
				FirstName: firstName,
				LastName:  "",
			}).To_InputContact(),
		},
	})
	if err != nil {
		return 0, 0, err
	}
	imp, ok := res.(*mtproto.TLContactsImportedContacts)
	if !ok {
		return 0, 0, fmt.Errorf("contacts.importContacts returned %T", res)
	}
	want := digitsOnly(phone)
	for _, user := range imp.GetUsers() {
		if user == nil || user.GetId() == 0 {
			continue
		}
		got := ""
		if user.GetPhone() != nil {
			got = digitsOnly(user.GetPhone().GetValue())
		}
		if got != "" && got != want {
			continue
		}
		var hash int64
		if user.GetAccessHash() != nil {
			hash = user.GetAccessHash().GetValue()
		}
		return user.GetId(), hash, nil
	}
	return 0, 0, fmt.Errorf("contacts.importContacts did not return %s", phone)
}
