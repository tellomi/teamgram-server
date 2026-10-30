package core

import (
	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/app/bff/tos/terms"
	"github.com/teamgram/teamgram-server/pkg/verify"

	"google.golang.org/grpc/status"
)

func maskPhone(phone string) string {
	if verify.IsEmail(phone) {
		return verify.MaskEmail(phone)
	}
	if len(phone) <= 4 {
		return "****"
	}
	return phone[:3] + "****" + phone[len(phone)-2:]
}

func hashPrefix(h string) string {
	if len(h) > 8 {
		return h[:8]
	}
	return h
}

func rpcErrorCode(err error) string {
	if err == nil {
		return ""
	}
	if st, ok := status.FromError(err); ok && st.Message() != "" {
		return st.Message()
	}
	return err.Error()
}

// newUserSignInResult is what auth.signIn returns when the phone has no user.
// Layer 104 and later (including 232) must be authorizationSignUpRequired so
// the client opens the name screen. Older layers get PHONE_NUMBER_UNOCCUPIED.
//
// terms_of_service is the same help.termsOfService document a later
// help.getTermsOfServiceUpdate would carry. Layer 232 has no
// help.getTermsOfService method. auth.signUp records acceptance of this id.
func newUserSignInResult(layer int32) (*mtproto.Auth_Authorization, error) {
	if layer >= 104 {
		return mtproto.MakeTLAuthAuthorizationSignUpRequired(&mtproto.Auth_Authorization{
			TermsOfService: terms.Current().SignupTerms(),
		}).To_Auth_Authorization(), nil
	}
	return nil, mtproto.ErrPhoneNumberUnoccupied
}
