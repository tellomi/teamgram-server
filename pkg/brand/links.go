// Copyright 2026 Tellomi Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0

// Package brand is the public link host used in server responses.
// Short links use tell.cc. Legal and support pages use tellomi.cn.
package brand

import (
	"os"
	"regexp"
	"strings"
)

const (
	// ShortHost is the t.me replacement.
	ShortHost = "tell.cc"
	// SiteHost serves privacy, terms, FAQ, support, and premium pages.
	SiteHost = "tellomi.cn"
	// LinkPrefix is help.getConfig me_url_prefix, with the trailing slash
	// clients append a username or invite hash to.
	LinkPrefix = "https://tell.cc/"

	PrivacyURL  = "https://tellomi.cn/privacy"
	TermsURL    = "https://tellomi.cn/terms"
	FAQURL      = "https://tellomi.cn/faq"
	SupportURL  = "https://tellomi.cn/support"
	PremiumURL  = "https://tellomi.cn/premium"
	AntispamURL = "https://tellomi.cn/antispam"
	CallPrefix  = "https://tell.cc/call/"
)

var (
	reSchemeShort = regexp.MustCompile(`(?i)https?://(?:www\.)?(?:t\.me|telegram\.me|telegram\.dog)`)
	reBareShort   = regexp.MustCompile(`(?i)(?:t\.me|telegram\.me|telegram\.dog)`)
	reOrg         = regexp.MustCompile(`(?i)https?://(?:www\.)?((?:[a-z0-9-]+\.)*)telegram\.org`)
	reOrgBare     = regexp.MustCompile(`(?i)telegram\.org`)
)

// Rewrite replaces Telegram short-link and site hosts in a response string.
// t.me, telegram.me, and telegram.dog become tell.cc.
// telegram.org, including subdomains, becomes tellomi.cn.
func Rewrite(s string) string {
	if s == "" || !mentionsTelegram(s) {
		return s
	}
	s = reSchemeShort.ReplaceAllString(s, "https://"+ShortHost)
	s = reOrg.ReplaceAllString(s, "https://${1}"+SiteHost)
	s = reBareShort.ReplaceAllString(s, ShortHost)
	s = reOrgBare.ReplaceAllString(s, SiteHost)
	return s
}

func mentionsTelegram(s string) bool {
	low := strings.ToLower(s)
	return strings.Contains(low, "t.me") || strings.Contains(low, "telegram.me") || strings.Contains(low, "telegram.dog") || strings.Contains(low, "telegram.org")
}

// LinkBase is the https host for invites, usernames, and message links.
// TELLOMI_LINK_BASE_URL and TEAMGRAM_TME_HOST override it, after branding.
func LinkBase() string {
	for _, key := range []string{"TELLOMI_LINK_BASE_URL", "TEAMGRAM_TME_HOST"} {
		v := strings.TrimSpace(os.Getenv(key))
		if v == "" {
			continue
		}
		v = strings.TrimRight(Rewrite(v), "/")
		if !strings.Contains(v, "://") {
			v = "https://" + strings.TrimLeft(v, "/")
		}
		return v
	}
	return strings.TrimRight(LinkPrefix, "/")
}

// InviteHash returns the hash from a tell.cc or legacy Telegram invite URL.
func InviteHash(link string) string {
	link = strings.TrimSpace(link)
	low := strings.ToLower(link)
	for _, host := range []string{ShortHost, "t.me", "telegram.me", "telegram.dog"} {
		for _, scheme := range []string{"https://", "http://"} {
			prefix := scheme + host + "/+"
			if strings.HasPrefix(low, prefix) {
				return link[len(prefix):]
			}
		}
	}
	return link
}
