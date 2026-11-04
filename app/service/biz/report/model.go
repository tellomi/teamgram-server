package report

import (
	"context"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/teamgram/proto/mtproto"
)

const (
	// AdminUserID is @admin. It is the same account announce uses for
	// service notices. Report receipts are ordinary private messages from
	// that account; login codes are a separate path.
	AdminUserID int64 = 777000

	SourceStory        = "stories.report"
	SourceMessages     = "messages.report"
	SourceReaction     = "messages.reportReaction"
	SourcePeer         = "account.reportPeer"
	SourceProfilePhoto = "account.reportProfilePhoto"
	SourceSpam         = "messages.reportSpam"
	SourceChannelSpam  = "channels.reportSpam"
	// SourcePost is a discover post. The feed service will call Submit with it.
	SourcePost = "discover.post"

	StatusPending    = "pending"
	StatusNeedsHuman = "needs_human"
	StatusViolation  = "violation"
	StatusClear      = "clear"

	noticeSubject = "subject"
)

// Submission is one stored report. Evidence is the content snapshot.
// Lang is the reporter's lang_code. An empty Lang uses the context value.
type Submission struct {
	Source         string
	Reporter       int64
	Lang           string
	PeerType       int32
	PeerID         int64
	SubjectUserID  int64
	Reason         string
	Message        string
	ObjectIDs      []int32
	PhotoID        int64
	ReactionPeerID int64
	Evidence       Evidence
	LegacyStoryID  int64
}

// Evidence is what a reviewer can read. Items are messages, stories, or a
// profile photo. Post is a discover post.
type Evidence struct {
	Items []Content
	Post  *Content
}

// Content is one piece of reported material.
type Content struct {
	Kind      string   `json:"kind"`
	ID        int64    `json:"id,omitempty"`
	Text      string   `json:"text,omitempty"`
	MediaKeys []string `json:"media_keys,omitempty"`
}

// Row is one reports table record.
type Row struct {
	ID             int64   `json:"id"`
	Source         string  `json:"source"`
	Reporter       int64   `json:"reporter_user_id"`
	Lang           string  `json:"lang_code"`
	PeerType       int32   `json:"peer_type"`
	PeerID         int64   `json:"peer_id"`
	SubjectUserID  int64   `json:"subject_user_id"`
	TargetKey      string  `json:"target_key"`
	Reason         string  `json:"reason"`
	Message        string  `json:"message"`
	ObjectIDs      []int32 `json:"object_ids,omitempty"`
	PhotoID        int64   `json:"photo_id"`
	ReactionPeerID int64   `json:"reaction_peer_id"`
	EvidenceKey    string  `json:"evidence_key"`
	Status         string  `json:"status"`
	LegacyStoryID  int64   `json:"legacy_story_id"`
	CreatedAt      int32   `json:"created_at"`
	DecidedAt      int32   `json:"decided_at"`
}

// Ticket is the public id, T plus the row id.
func Ticket(id int64) string {
	return "T" + strconv.FormatInt(id, 10)
}

// TicketOf is the ticket on a row.
func (r *Row) Ticket() string {
	if r == nil {
		return ""
	}
	return Ticket(r.ID)
}

func (in Submission) validate() error {
	if in.Reporter == 0 {
		return mtproto.ErrAuthKeyUnregistered
	}
	if strings.TrimSpace(in.Source) == "" {
		return mtproto.ErrInputRequestInvalid
	}
	if len(in.Message) > 1024 {
		return mtproto.ErrMessageTooLong
	}
	return nil
}

func evidenceEmpty(e Evidence) bool {
	return len(e.Items) == 0 && e.Post == nil
}

// TargetKey groups reports of one piece of content so a decision can notify once.
func TargetKey(source string, peerType int32, peerID int64, objectIDs []int32, photoID, reactionPeer int64) string {
	ids := joinIDs(normalizeIDs(objectIDs))
	switch source {
	case SourceStory:
		return "story:" + itoa(int64(peerType)) + ":" + itoa(peerID) + ":" + ids
	case SourceReaction:
		return "reaction:" + itoa(int64(peerType)) + ":" + itoa(peerID) + ":" + ids + ":" + itoa(reactionPeer)
	case SourceProfilePhoto:
		return "photo:" + itoa(int64(peerType)) + ":" + itoa(peerID) + ":" + itoa(photoID)
	case SourceChannelSpam:
		return "cspam:" + itoa(int64(peerType)) + ":" + itoa(peerID) + ":" + ids
	case SourcePost:
		return "post:" + itoa(peerID)
	case SourceMessages:
		if ids != "" {
			return "msg:" + itoa(int64(peerType)) + ":" + itoa(peerID) + ":" + ids
		}
	}
	return "peer:" + itoa(int64(peerType)) + ":" + itoa(peerID)
}

func normalizeIDs(ids []int32) []int32 {
	if len(ids) == 0 {
		return nil
	}
	cp := append([]int32(nil), ids...)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	out := make([]int32, 0, len(cp))
	var prev int32
	seen := false
	for _, id := range cp {
		if id <= 0 {
			continue
		}
		if seen && id == prev {
			continue
		}
		out = append(out, id)
		prev = id
		seen = true
	}
	return out
}

func joinIDs(ids []int32) string {
	if len(ids) == 0 {
		return ""
	}
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(int64(id), 10)
	}
	return strings.Join(parts, ",")
}

func splitIDs(s string) []int32 {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]int32, 0, len(parts))
	for _, p := range parts {
		n, err := strconv.ParseInt(strings.TrimSpace(p), 10, 32)
		if err != nil || n <= 0 {
			continue
		}
		out = append(out, int32(n))
	}
	return normalizeIDs(out)
}

func clipRunes(s string, n int) string {
	if n <= 0 || s == "" || utf8.RuneCountInString(s) <= n {
		return s
	}
	i := 0
	for count := 0; i < len(s) && count < n; count++ {
		_, size := utf8.DecodeRuneInString(s[i:])
		i += size
	}
	return s[:i]
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// LangLookup reads a user's lang_code. A miss returns an empty string.
type LangLookup interface {
	Lang(ctx context.Context, userID int64) string
}

type langCtxKey struct{}

// WithLang stores the caller's lang_code for Submit.
func WithLang(ctx context.Context, lang string) context.Context {
	lang = strings.TrimSpace(lang)
	if lang == "" || ctx == nil {
		return ctx
	}
	return context.WithValue(ctx, langCtxKey{}, lang)
}

// LangFrom reads the lang_code stored by WithLang.
func LangFrom(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	s, _ := ctx.Value(langCtxKey{}).(string)
	return s
}
