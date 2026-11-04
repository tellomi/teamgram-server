package contentsafety

import (
	"bytes"
	"encoding/json"
	"strings"
	"unicode"
)

const (
	// Allowed means the text may be saved.
	Allowed = "allowed"
	// Blocked means the moderation service refused the text.
	Blocked = "blocked"
	// Fallback means the service was not used. The caller allows the text.
	Fallback = "fallback"
)

type envelope struct {
	Code      flexString      `json:"Code"`
	Message   string          `json:"Message"`
	RequestID string          `json:"RequestId"`
	Data      json.RawMessage `json:"Data"`
}

type flexString string

func (f *flexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*f = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*f = flexString(s)
		return nil
	}
	*f = flexString(b)
	return nil
}

type moderationData struct {
	Labels    string          `json:"labels"`
	LabelsAlt string          `json:"Labels"`
	Risk      string          `json:"riskLevel"`
	RiskAlt   string          `json:"RiskLevel"`
	Result    []moderationHit `json:"Result"`
	ResultAlt []moderationHit `json:"result"`
}

type moderationHit struct {
	Label    string `json:"Label"`
	LabelAlt string `json:"label"`
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func realLabels(parts ...string) string {
	var out []string
	seen := map[string]bool{}
	for _, part := range parts {
		for _, p := range strings.Split(part, ",") {
			p = strings.TrimSpace(p)
			switch strings.ToLower(p) {
			case "", "nonlabel", "non_label":
				continue
			}
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return strings.Join(out, ",")
}

// interpret reads a Green response. A non-decision is Fallback.
// statusCode is the HTTP status. body is the raw JSON.
func interpret(statusCode int, body []byte) (outcome, labels, risk, code, requestID string) {
	if statusCode < 200 || statusCode >= 300 {
		return Fallback, "", "", "", ""
	}
	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return Fallback, "", "", "", ""
	}
	code = string(env.Code)
	requestID = env.RequestID
	if code != "200" {
		return Fallback, "", "", code, requestID
	}
	data, ok := decodeData(env.Data)
	if !ok {
		return Fallback, "", "", code, requestID
	}
	risk = data.Risk
	if risk == "" {
		risk = data.RiskAlt
	}
	var hits []string
	hits = append(hits, data.Labels, data.LabelsAlt)
	for _, hit := range append(data.Result, data.ResultAlt...) {
		if hit.Label != "" {
			hits = append(hits, hit.Label)
		}
		if hit.LabelAlt != "" {
			hits = append(hits, hit.LabelAlt)
		}
	}
	labels = realLabels(hits...)
	switch strings.ToLower(strings.TrimSpace(risk)) {
	case "high", "medium":
		return Blocked, labels, risk, code, requestID
	}
	if labels != "" {
		return Blocked, labels, risk, code, requestID
	}
	return Allowed, "", risk, code, requestID
}

func decodeData(raw json.RawMessage) (moderationData, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return moderationData{}, true
	}
	if raw[0] == '"' {
		var encoded string
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return moderationData{}, false
		}
		raw = []byte(encoded)
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return moderationData{}, true
	}
	var data moderationData
	if err := json.Unmarshal(raw, &data); err != nil {
		return moderationData{}, false
	}
	return data, true
}
