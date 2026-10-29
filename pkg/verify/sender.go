package verify

import (
	"net/http"
	"time"

	"github.com/teamgram/teamgram-server/pkg/verify/aliyun"
)

// NewEmailSender returns the email CodeSender for cfg.
// An unset provider returns DisabledEmail, which refuses to send.
// "log" prints the code. "aliyun_dm" builds a DirectMail client but does not
// dial until SendCode. Authorization RPCs do not call this yet.
func NewEmailSender(cfg EmailConfig) CodeSender {
	switch cfg.Provider {
	case ProviderLog:
		return LogEmail{}
	case ProviderAliyunDM:
		return &aliyun.DirectMailClient{
			AccessKeyID:     cfg.AccessKeyID,
			AccessKeySecret: cfg.AccessKeySecret,
			AccountName:     cfg.AccountName,
			FromAlias:       cfg.FromAlias,
			Endpoint:        cfg.Endpoint,
			HTTP:            &http.Client{Timeout: 10 * time.Second},
		}
	default:
		return DisabledEmail{}
	}
}
