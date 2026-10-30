package verify

import (
	"net/http"
	"time"

	"github.com/teamgram/teamgram-server/pkg/verify/postal"
)

// NewEmailSender returns the email CodeSender for cfg.
// An empty provider returns DisabledEmail.
// "log" prints the code. "postal" builds an HTTP client and does not dial
// until SendCode. Aliyun DirectMail is not selected.
func NewEmailSender(cfg EmailConfig) CodeSender {
	switch cfg.Provider {
	case ProviderLog:
		return LogEmail{}
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
