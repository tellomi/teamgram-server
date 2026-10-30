package verify

import (
	"net/http"
	"time"

	"github.com/teamgram/teamgram-server/pkg/verify/postal"
	"github.com/teamgram/teamgram-server/pkg/verify/resend"
)

// NewEmailSender returns the email CodeSender for cfg.
// An empty provider returns DisabledEmail.
// "log" prints the code. "resend" and "postal" build HTTP clients and do not
// dial until SendCode. Aliyun DirectMail is not selected.
func NewEmailSender(cfg EmailConfig) CodeSender {
	switch cfg.Provider {
	case ProviderLog:
		return LogEmail{}
	case ProviderResend:
		return &resend.Client{
			APIKey:   cfg.ResendAPIKey,
			From:     cfg.From,
			FromName: cfg.FromName,
			HTTP:     &http.Client{Timeout: 10 * time.Second},
		}
	case ProviderPostal:
		return &postal.Client{
			BaseURL:  cfg.PostalBaseURL,
			APIKey:   cfg.PostalAPIKey,
			From:     cfg.From,
			FromName: cfg.FromName,
			HTTP:     &http.Client{Timeout: 10 * time.Second},
		}
	default:
		return DisabledEmail{}
	}
}
