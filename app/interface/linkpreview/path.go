package linkpreview

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	opUser     = "user"
	opInvite   = "invite"
	opMessage  = "message"
	opFeed     = "feed"
	opCMessage = "cmessage"
	opStickers = "stickers"
	opEmoji    = "emoji"
	opProxy    = "proxy"
	opSocks    = "socks"
	opShare    = "share"
	opBoost    = "boost"
	opStory    = "story"
)

const (
	formProfile = "profile"
	formPost    = "post"
	formWeb     = "web"
	formC       = "c"
	formStory   = "story"
	formBoost   = "boost"
)

var (
	usernameRE = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_]{2,31}$`)
	hashRE     = regexp.MustCompile(`^[A-Za-z0-9_-]{8,128}$`)
	shortRE    = regexp.MustCompile(`^[A-Za-z0-9_]{1,64}$`)
	hostRE     = regexp.MustCompile(`^[A-Za-z0-9._:\-\[\]]{1,253}$`)
	secretRE   = regexp.MustCompile(`^[A-Za-z0-9_+\-/=]{1,256}$`)
	startRE    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
)

type route struct {
	op       string
	form     string
	username string
	hash     string
	short    string
	id       int64
	msgID    int32
	storyID  int32
	query    url.Values
}

func parseRequest(raw string, extra url.Values) (*route, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 2048 || strings.ContainsRune(raw, 0) {
		return nil, errInvalid
	}
	path, q, err := splitPathQuery(raw)
	if err != nil {
		return nil, errInvalid
	}
	if extra != nil {
		for k, vs := range extra {
			if strings.EqualFold(k, "path") || len(vs) == 0 {
				continue
			}
			if q.Get(k) == "" {
				q.Set(k, vs[0])
			}
		}
	}
	segs, err := splitSegments(path)
	if err != nil || len(segs) == 0 || len(segs) > 4 {
		return nil, errInvalid
	}
	return dispatch(segs, q)
}

func splitPathQuery(raw string) (string, url.Values, error) {
	q := url.Values{}
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" || !allowedHost(u.Host) {
			return "", nil, errInvalid
		}
		return u.Path, u.Query(), nil
	}
	path := raw
	if i := strings.IndexByte(raw, '?'); i >= 0 {
		path = raw[:i]
		parsed, err := url.ParseQuery(raw[i+1:])
		if err != nil {
			return "", nil, err
		}
		q = parsed
	}
	return path, q, nil
}

func allowedHost(host string) bool {
	host = strings.ToLower(host)
	if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host, "]") {
		host = host[:i]
	}
	host = strings.TrimPrefix(host, "www.")
	switch host {
	case "t.me", "telegram.me", "telegram.dog", "tell.cc":
		return true
	default:
		return false
	}
}

func splitSegments(path string) ([]string, error) {
	path = strings.TrimSpace(path)
	if path == "" || strings.Contains(path, "\\") {
		return nil, errInvalid
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	var segs []string
	for _, p := range strings.Split(path, "/") {
		if p == "" {
			continue
		}
		dec, err := url.PathUnescape(p)
		if err != nil || dec == "" || dec == "." || dec == ".." || strings.Contains(dec, "/") {
			return nil, errInvalid
		}
		segs = append(segs, dec)
	}
	return segs, nil
}

func dispatch(segs []string, q url.Values) (*route, error) {
	head := segs[0]
	if strings.HasPrefix(head, "+") {
		if len(segs) != 1 {
			return nil, errInvalid
		}
		hash := strings.TrimPrefix(head, "+")
		if !hashRE.MatchString(hash) {
			return nil, errInvalid
		}
		return &route{op: opInvite, hash: hash}, nil
	}
	switch strings.ToLower(head) {
	case "joinchat":
		if len(segs) != 2 || !hashRE.MatchString(segs[1]) {
			return nil, errInvalid
		}
		return &route{op: opInvite, hash: segs[1]}, nil
	case "addstickers":
		if len(segs) != 2 || !shortRE.MatchString(segs[1]) {
			return nil, errInvalid
		}
		return &route{op: opStickers, short: segs[1]}, nil
	case "addemoji":
		if len(segs) != 2 || !shortRE.MatchString(segs[1]) {
			return nil, errInvalid
		}
		return &route{op: opEmoji, short: segs[1]}, nil
	case "proxy":
		if len(segs) != 1 {
			return nil, errInvalid
		}
		return &route{op: opProxy, query: q}, nil
	case "socks":
		if len(segs) != 1 {
			return nil, errInvalid
		}
		return &route{op: opSocks, query: q}, nil
	case "share":
		if len(segs) != 2 || !strings.EqualFold(segs[1], "url") {
			return nil, errInvalid
		}
		return &route{op: opShare, query: q}, nil
	case "boost":
		if len(segs) == 1 {
			return &route{op: opBoost, form: formBoost}, nil
		}
		if len(segs) == 2 && validUsername(segs[1]) {
			return &route{op: opBoost, form: formBoost, username: segs[1]}, nil
		}
		return nil, errInvalid
	case "s":
		if len(segs) < 2 || len(segs) > 3 || !validUsername(segs[1]) {
			return nil, errInvalid
		}
		r := &route{op: opFeed, form: formWeb, username: segs[1]}
		if len(segs) == 3 {
			id, err := parsePositive32(segs[2])
			if err != nil {
				return nil, errInvalid
			}
			r.op = opMessage
			r.msgID = id
		}
		return r, nil
	case "c":
		if len(segs) != 3 {
			return nil, errInvalid
		}
		id, err := parseChannelPathID(segs[1])
		if err != nil {
			return nil, errInvalid
		}
		mid, err := parsePositive32(segs[2])
		if err != nil {
			return nil, errInvalid
		}
		return &route{op: opCMessage, form: formC, id: id, msgID: mid}, nil
	default:
		if !validUsername(head) {
			return nil, errInvalid
		}
		if len(segs) == 1 {
			return &route{op: opUser, form: formProfile, username: head, query: q}, nil
		}
		if len(segs) == 2 && strings.EqualFold(segs[1], "boost") {
			return &route{op: opBoost, form: formBoost, username: head}, nil
		}
		if len(segs) == 3 && strings.EqualFold(segs[1], "s") {
			sid, err := parsePositive32(segs[2])
			if err != nil {
				return nil, errInvalid
			}
			return &route{op: opStory, form: formStory, username: head, storyID: sid}, nil
		}
		if len(segs) == 2 {
			mid, err := parsePositive32(segs[1])
			if err != nil {
				return nil, errInvalid
			}
			return &route{op: opMessage, form: formPost, username: head, msgID: mid}, nil
		}
		return nil, errInvalid
	}
}

func validUsername(name string) bool {
	return usernameRE.MatchString(name) && !strings.HasSuffix(name, "_")
}

func parsePositive32(s string) (int32, error) {
	if s == "" || (len(s) > 1 && s[0] == '0') || strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, errInvalid
	}
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil || n <= 0 {
		return 0, errInvalid
	}
	return int32(n), nil
}

func parseChannelPathID(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "-100") && len(s) > 4 {
		s = s[4:]
	}
	if s == "" || strings.Contains(s, "-") || (len(s) > 1 && s[0] == '0') {
		return 0, errInvalid
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil || n <= 0 {
		return 0, errInvalid
	}
	return n, nil
}

func validHost(host string) bool {
	if !hostRE.MatchString(host) || strings.Contains(host, "..") {
		return false
	}
	return true
}

func validSecret(secret string) bool {
	return secretRE.MatchString(secret)
}

func startFrom(q url.Values) (*Start, error) {
	if q == nil {
		return nil, nil
	}
	for _, param := range []string{"start", "startgroup", "startchannel", "startattach"} {
		v := q.Get(param)
		if v == "" {
			continue
		}
		if !startRE.MatchString(v) {
			return nil, errInvalid
		}
		return &Start{Param: param, Value: v}, nil
	}
	return nil, nil
}
