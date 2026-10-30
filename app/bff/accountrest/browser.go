package accountrest

import (
	"net/url"
	"strings"
	"sync"

	"github.com/teamgram/proto/mtproto"
)

// Web browser settings are per user.
// https://core.telegram.org/method/account.getWebBrowserSettings is not on
// the public method index; the constructors are layer 232.
// account.getWebBrowserSettings#56655768 hash:long = account.WebBrowserSettings;
// account.updateWebBrowserSettings#9adf82fe flags:# open_external_browser:flags.0?true display_close_button:flags.1?true = account.WebBrowserSettings;
// account.toggleWebBrowserSettingsException#60ed4229 flags:# delete:flags.1?true open_external_browser:flags.0?Bool url:string = Updates;
// account.deleteWebBrowserSettingsExceptions#86a0765d = account.WebBrowserSettings;

type browserState struct {
	openExternal bool
	displayClose bool
	external     []string
	inapp        []string
}

type browserStore struct {
	mu    sync.Mutex
	users map[int64]*browserState
}

var browsers = &browserStore{users: map[int64]*browserState{}}

func resetBrowsers() {
	browsers.mu.Lock()
	browsers.users = map[int64]*browserState{}
	browsers.mu.Unlock()
}

func (s *browserStore) state(userID int64) *browserState {
	st := s.users[userID]
	if st == nil {
		st = &browserState{}
		s.users[userID] = st
	}
	return st
}

// GetWebBrowserSettings returns the settings, or account.webBrowserSettingsNotModified
// when hash is the current non-zero hash.
func GetWebBrowserSettings(userID, hash int64) (*mtproto.Account_WebBrowserSettings, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	browsers.mu.Lock()
	defer browsers.mu.Unlock()
	st := browsers.state(userID)
	current := browserHash(st)
	if hash != 0 && hash == current {
		return mtproto.MakeTLAccountWebBrowserSettingsNotModified(&mtproto.Account_WebBrowserSettings{}).To_Account_WebBrowserSettings(), nil
	}
	return browserSettings(st, current), nil
}

// UpdateWebBrowserSettings replaces the two boolean flags. The flag is the value.
func UpdateWebBrowserSettings(userID int64, openExternal, displayClose bool) (*mtproto.Account_WebBrowserSettings, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	browsers.mu.Lock()
	defer browsers.mu.Unlock()
	st := browsers.state(userID)
	st.openExternal = openExternal
	st.displayClose = displayClose
	return browserSettings(st, browserHash(st)), nil
}

// ToggleWebBrowserException adds or deletes a domain exception.
// openExternal true puts the domain on the external list; false puts it on the in-app list.
// delete removes it from both. A URL without a host is 400 URL_INVALID.
// The result is updates containing updateWebBrowserException.
func ToggleWebBrowserException(userID int64, rawURL string, delete bool, openExternal *mtproto.Bool) (*mtproto.Updates, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	domain, err := browserDomain(rawURL)
	if err != nil {
		return nil, err
	}
	browsers.mu.Lock()
	defer browsers.mu.Unlock()
	st := browsers.state(userID)
	st.external = dropDomain(st.external, domain)
	st.inapp = dropDomain(st.inapp, domain)
	if !delete {
		if mtproto.FromBool(openExternal) {
			st.external = append(st.external, domain)
		} else {
			st.inapp = append(st.inapp, domain)
		}
	}
	ex := mtproto.MakeTLWebDomainException(&mtproto.WebDomainException{
		Domain: domain,
		Url:    strings.TrimSpace(rawURL),
		Title:  "",
	}).To_WebDomainException()
	up := mtproto.MakeTLUpdateWebBrowserException(&mtproto.Update{
		Delete:                       delete,
		OpenExternalBrowser_FLAGBOOL: openExternal,
		Exception_WEBDOMAINEXCEPTION: ex,
	}).To_Update()
	return mtproto.MakeUpdatesByUpdates(up), nil
}

// DeleteWebBrowserExceptions clears both exception lists and returns the settings.
func DeleteWebBrowserExceptions(userID int64) (*mtproto.Account_WebBrowserSettings, error) {
	if userID == 0 {
		return nil, mtproto.ErrAuthKeyUnregistered
	}
	browsers.mu.Lock()
	defer browsers.mu.Unlock()
	st := browsers.state(userID)
	st.external = nil
	st.inapp = nil
	return browserSettings(st, browserHash(st)), nil
}

func browserSettings(st *browserState, hash int64) *mtproto.Account_WebBrowserSettings {
	return mtproto.MakeTLAccountWebBrowserSettings(&mtproto.Account_WebBrowserSettings{
		OpenExternalBrowser: st.openExternal,
		DisplayCloseButton:  st.displayClose,
		ExternalExceptions:  domainExceptions(st.external),
		InappExceptions:     domainExceptions(st.inapp),
		Hash:                hash,
	}).To_Account_WebBrowserSettings()
}

func domainExceptions(domains []string) []*mtproto.WebDomainException {
	out := make([]*mtproto.WebDomainException, 0, len(domains))
	for _, domain := range domains {
		out = append(out, mtproto.MakeTLWebDomainException(&mtproto.WebDomainException{
			Domain: domain,
			Url:    "https://" + domain,
			Title:  "",
		}).To_WebDomainException())
	}
	return out
}

func dropDomain(list []string, domain string) []string {
	next := list[:0]
	for _, item := range list {
		if !strings.EqualFold(item, domain) {
			next = append(next, item)
		}
	}
	if len(next) == 0 {
		return nil
	}
	return next
}

func browserDomain(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", mtproto.ErrUrlInvalid
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Hostname() == "" {
		if !strings.Contains(raw, "://") {
			parsed, err = url.Parse("https://" + raw)
		}
	}
	if err != nil || parsed.Hostname() == "" {
		return "", mtproto.ErrUrlInvalid
	}
	if parsed.Scheme != "" && parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", mtproto.ErrUrlInvalid
	}
	return strings.ToLower(parsed.Hostname()), nil
}

func browserHash(st *browserState) int64 {
	h := uint64(1469598103934665603)
	if st.openExternal {
		h = h*1099511628211 ^ 1
	}
	if st.displayClose {
		h = h*1099511628211 ^ 2
	}
	for _, domain := range st.external {
		h = hashText(h, domain)
	}
	h ^= 0xff
	for _, domain := range st.inapp {
		h = hashText(h, domain)
	}
	out := int64(h & 0x7fffffffffffffff)
	if out == 0 {
		return 1
	}
	return out
}

func hashText(h uint64, text string) uint64 {
	for _, c := range text {
		h = h*1099511628211 ^ uint64(c)
	}
	return h*1099511628211 ^ 0x1f
}
