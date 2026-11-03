// Package discoverrel answers content-engine visibility checks.
// can_view calls POST /internal/v1/relations/check on each read.
// Contacts and close friends come from the author's chat rows, not from a
// friend_edge snapshot. Selected users stay on the audience token.
package discoverrel

import "encoding/base64"

const (
	// EnvEnabled turns the internal relation API on. Unset or any other value is off.
	EnvEnabled = "TELLOMI_DISCOVER_RELATIONS"
	// EnvKey is standard base64 of the bearer secret the engine sends.
	// Decoded length is at least 32 bytes.
	EnvKey = "TELLOMI_DISCOVER_RELATION_KEY"
	// EnvAddr is the listen address. Empty means 127.0.0.1:8812. "off" does not listen.
	EnvAddr = "TELLOMI_DISCOVER_RELATION_ADDR"

	defaultAddr = "127.0.0.1:8812"
	maxAuthors  = 100
)

// Edge is one viewer's relationship to an author, from the author's
// contact row. A removed contact is not an edge.
type Edge struct {
	Contact     bool
	CloseFriend bool
}

// Result is one author in the request order.
type Result struct {
	Author      string `json:"author"`
	Known       bool   `json:"known"`
	Contact     bool   `json:"contact"`
	CloseFriend bool   `json:"close_friend"`
}

// Answer is the HTTP body.
type Answer struct {
	ViewerKnown bool     `json:"viewer_known"`
	Results     []Result `json:"results"`
}

// Evaluate applies live edges. Unknown people are not contacts and not
// close friends. The author looking up themselves is not a contact of
// themselves; the engine allows the author through before it calls.
func Evaluate(viewerSub string, viewerID int64, authors []string, users map[string]int64, edges map[int64]Edge) Answer {
	out := Answer{
		ViewerKnown: viewerID > 0,
		Results:     make([]Result, len(authors)),
	}
	for i, author := range authors {
		row := Result{Author: author}
		authorID := users[author]
		row.Known = authorID > 0
		if out.ViewerKnown && row.Known && authorID != viewerID && author != viewerSub {
			edge := edges[authorID]
			row.Contact = edge.Contact
			row.CloseFriend = edge.CloseFriend && edge.Contact
		}
		out.Results[i] = row
	}
	return out
}

func subKey(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}
