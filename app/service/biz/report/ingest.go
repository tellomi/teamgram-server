package report

import (
	"context"
	"errors"
	"strings"

	"github.com/teamgram/proto/mtproto"
)

// DiscoverReport is one content-engine forward of a Discover report.
// Reason is a Telegram report option id.
type DiscoverReport struct {
	Reporter  int64
	Author    int64
	PostID    string
	CommentID string
	Reason    string
	Comment   string
	Lang      string
}

// IngestDiscover writes one Discover report into the unified table and sends
// the @admin receipt. A second call for the same reporter and target returns
// the existing row and does not send another receipt.
func (s *Service) IngestDiscover(ctx context.Context, in DiscoverReport) (*Row, bool, error) {
	if s == nil || s.store == nil {
		return nil, false, errUnavailable
	}
	source, target := discoverTarget(in.PostID, in.CommentID)
	key := discoverDedupe(in.Reporter, target)
	existing, err := s.store.FindDedupe(ctx, key)
	if err != nil {
		return nil, false, err
	}
	if existing != nil {
		return existing, true, nil
	}
	ev := Evidence{Post: &Content{Kind: "post", Ref: in.PostID}}
	if in.CommentID != "" {
		ev.Items = []Content{{Kind: "comment", Ref: in.CommentID}}
	}
	row, err := s.Submit(ctx, Submission{
		Source:        source,
		Reporter:      in.Reporter,
		Lang:          in.Lang,
		PeerType:      mtproto.PEER_USER,
		PeerID:        in.Author,
		SubjectUserID: in.Author,
		TargetKey:     target,
		Reason:        in.Reason,
		Message:       in.Comment,
		Evidence:      ev,
		DedupeKey:     key,
	})
	if errors.Is(err, ErrDuplicate) {
		existing, ferr := s.store.FindDedupe(ctx, key)
		return existing, true, ferr
	}
	return row, false, err
}

// Moderate classifies keyword-hit post text. The public verdict is
// violation, ok, or uncertain. A missing classifier returns uncertain and
// does not call a model. Nothing is written to the reports table.
func (s *Service) Moderate(ctx context.Context, text string, media []string) string {
	if s == nil || s.reviewer == nil {
		return VerdictUncertain
	}
	row := &Row{Source: SourcePost, Message: text}
	ev := Evidence{Post: &Content{Kind: "post", Text: text, MediaKeys: append([]string(nil), media...)}}
	switch s.reviewer.Review(ctx, row, ev) {
	case StatusViolation:
		return VerdictViolation
	case StatusClear:
		return VerdictOK
	default:
		return VerdictUncertain
	}
}

const (
	VerdictViolation = "violation"
	VerdictOK        = "ok"
	VerdictUncertain = "uncertain"

	sourceComment = "discover.comment"
)

func discoverTarget(postID, commentID string) (source, target string) {
	if commentID != "" {
		return sourceComment, "comment:" + postID + ":" + commentID
	}
	return SourcePost, "post:" + postID
}

func discoverDedupe(reporter int64, target string) string {
	return itoa(reporter) + ":" + target
}

func validDiscoverID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == ':' || r == '-':
		default:
			return false
		}
	}
	return true
}

func validDiscoverReason(reason string) bool {
	switch reason {
	case "spam", "violence", "pornography", "child", "drugs", "personal", "other":
		return true
	default:
		return false
	}
}

func validMediaKey(key string) bool {
	if key == "" || len(key) > 255 || strings.Contains(key, "..") || strings.HasPrefix(key, "/") {
		return false
	}
	for _, r := range key {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		case r == '.' || r == '_' || r == ':' || r == '-' || r == '/':
		default:
			return false
		}
	}
	return true
}

var errUnavailable = errString("report: service is not configured")

type errString string

func (e errString) Error() string { return string(e) }
