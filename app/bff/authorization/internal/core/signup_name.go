package core

import (
	"context"

	"github.com/teamgram/proto/mtproto"
	"github.com/teamgram/teamgram-server/pkg/usernamepolicy"
	"google.golang.org/grpc/status"
)

// rejectSignupName applies the display-name lexicon to a new account.
// Chat message bodies are not checked. An empty last name is allowed.
// peer id 0 is a user that does not exist yet, so a reserved official name
// is not treated as that user's own name.
func rejectSignupName(ctx context.Context, first, last string) error {
	if err := usernamepolicy.RejectDisplay(ctx, mtproto.PEER_USER, 0, first); err != nil {
		return err
	}
	if err := usernamepolicy.RejectDisplay(ctx, mtproto.PEER_USER, 0, last); err != nil {
		if status.Convert(err).Message() == "FIRSTNAME_INVALID" {
			return mtproto.ErrLastnameInvalid
		}
		return err
	}
	return nil
}
