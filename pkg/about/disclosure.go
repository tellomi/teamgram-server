// Package about publishes the strings the About page and the
// personalized-recommendation switch read from help.getAppConfig.
//
// The switch is advertised only when TELLOMI_ABOUT_DISCLOSURE=1 and a
// public algorithm filing number is set. Official clients ignore these keys.
// This process does not store a per-user choice and does not rank the feed.
package about

import (
	"os"
	"strings"
	"unicode/utf8"
)

const (
	// EnvEnabled gates every key in this package. Only the value 1 turns it on.
	EnvEnabled = "TELLOMI_ABOUT_DISCLOSURE"
	// EnvAlgorithmFiling is the public algorithm filing number (D-59).
	EnvAlgorithmFiling = "TELLOMI_ALGORITHM_FILING_NUMBER"
	// EnvAIModel is the model name shown on the About page (D-50).
	EnvAIModel = "TELLOMI_AI_PUBLIC_MODEL"
	// EnvAIRegistration is the generative-AI registration number (D-50).
	EnvAIRegistration = "TELLOMI_AI_REGISTRATION_NUMBER"

	maxRunes = 128
)

// PersonalizedRecommendDefault is tellomi_personalized_recommend_default.
// False means clients leave the privacy switch hidden. True means show it,
// defaulting to on (D-58). True requires the disclosure flag and a filing number.
func PersonalizedRecommendDefault() bool {
	return AlgorithmFilingNumber() != ""
}

// AlgorithmFilingNumber is tellomi_algorithm_filing_number, or empty when
// the key should be omitted.
func AlgorithmFilingNumber() string {
	if os.Getenv(EnvEnabled) != "1" {
		return ""
	}
	return publicText(os.Getenv(EnvAlgorithmFiling))
}

// AIModelName is tellomi_ai_model_name, or empty when the key should be omitted.
func AIModelName() string {
	if os.Getenv(EnvEnabled) != "1" {
		return ""
	}
	return publicText(os.Getenv(EnvAIModel))
}

// AIRegistrationNumber is tellomi_ai_registration_number, or empty when
// the key should be omitted.
func AIRegistrationNumber() string {
	if os.Getenv(EnvEnabled) != "1" {
		return ""
	}
	return publicText(os.Getenv(EnvAIRegistration))
}

// HashFragment is mixed into the help.getAppConfig hash. It changes when the
// flag or any published string changes, and stays the same while the flag is off.
func HashFragment() string {
	on := "0"
	if os.Getenv(EnvEnabled) == "1" {
		on = "1"
	}
	return "about=" + on +
		" filing=" + AlgorithmFilingNumber() +
		" model=" + AIModelName() +
		" aireg=" + AIRegistrationNumber()
}

// publicText is a trimmed disclosure string. Control characters, a Telegram
// host, and a value longer than maxRunes are treated as unset.
func publicText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || utf8.RuneCountInString(s) > maxRunes {
		return ""
	}
	for _, r := range s {
		if r < 0x20 || r == 0x7f {
			return ""
		}
	}
	low := strings.ToLower(s)
	if strings.Contains(low, "telegram") || strings.Contains(low, "t.me") {
		return ""
	}
	return s
}
