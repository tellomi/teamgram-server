package report

import (
	"context"
	"encoding/json"
	"strings"
)

// Completer is the model call. *tellomiai.Client implements it.
// The user-facing notice never includes the completion text.
type Completer interface {
	CompletePrompt(ctx context.Context, model, system, prompt string) (string, int, error)
}

// reviewSystem asks for one of three results. It does not name a model,
// and the notice copy is chosen from copy.go, not from this text.
const reviewSystem = "Classify the report. Reply with one JSON object and nothing else. " +
	"Use {\"result\":\"violation\"} when the content breaks platform rules, " +
	"{\"result\":\"clear\"} when it does not, or {\"result\":\"needs_human\"} when you are not sure."

// Reviewer classifies one report. A nil reviewer means the row waits for a person.
type Reviewer struct {
	Complete Completer
	Model    string
}

// NewReviewer wraps a completer. A nil completer returns nil.
func NewReviewer(c Completer) *Reviewer {
	if c == nil {
		return nil
	}
	return &Reviewer{Complete: c}
}

// Review returns violation, clear, or needs_human. A transport problem
// returns needs_human so a person still sees the row.
func (r *Reviewer) Review(ctx context.Context, row *Row, ev Evidence) string {
	if r == nil || r.Complete == nil || row == nil {
		return StatusNeedsHuman
	}
	text, _, err := r.Complete.CompletePrompt(ctx, r.Model, reviewSystem, reviewPrompt(row, ev))
	if err != nil {
		return StatusNeedsHuman
	}
	return parseVerdict(text)
}

func reviewPrompt(row *Row, ev Evidence) string {
	var b strings.Builder
	b.WriteString("Source: ")
	b.WriteString(row.Source)
	b.WriteString("\nReason: ")
	b.WriteString(row.Reason)
	if row.Message != "" {
		b.WriteString("\nNote: ")
		b.WriteString(clipRunes(row.Message, 500))
	}
	for _, item := range ev.Items {
		writeContent(&b, item)
	}
	if ev.Post != nil {
		writeContent(&b, *ev.Post)
	}
	return clipRunes(b.String(), 4000)
}

func writeContent(b *strings.Builder, item Content) {
	b.WriteString("\nContent: ")
	if item.Text != "" {
		b.WriteString(clipRunes(item.Text, 500))
	}
	if len(item.MediaKeys) > 0 {
		b.WriteString("\nMedia: ")
		b.WriteString(strings.Join(item.MediaKeys, " "))
	}
}

func parseVerdict(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "```json")
	text = strings.TrimPrefix(text, "```")
	text = strings.TrimSuffix(text, "```")
	text = strings.TrimSpace(text)
	start := strings.IndexByte(text, '{')
	end := strings.LastIndexByte(text, '}')
	if start >= 0 && end > start {
		var body struct {
			Result string `json:"result"`
		}
		if json.Unmarshal([]byte(text[start:end+1]), &body) == nil {
			switch strings.ToLower(strings.TrimSpace(body.Result)) {
			case StatusViolation, "no_violation":
				if strings.EqualFold(body.Result, "no_violation") {
					return StatusClear
				}
				return StatusViolation
			case StatusClear:
				return StatusClear
			case StatusNeedsHuman, "human":
				return StatusNeedsHuman
			}
		}
	}
	switch strings.ToLower(strings.Trim(text, "\"' ")) {
	case StatusViolation:
		return StatusViolation
	case StatusClear, "no_violation":
		return StatusClear
	case StatusNeedsHuman:
		return StatusNeedsHuman
	default:
		return StatusNeedsHuman
	}
}
