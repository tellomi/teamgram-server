// Package main regenerates the official-client API coverage matrix.
//
//	go run ./tools/apicoverage
//	go run ./tools/apicoverage -extract -ios ... -android ... -desktop ... -macos ...
//	go run ./tools/apicoverage -check
//
// Server status is always read from this repo. Client call sites come from
// tools/apicoverage/data/clients.json so CI does not clone the Telegram apps.
package main

// Status values written into the matrix.
const (
	statusImplemented        = "implemented"
	statusStub               = "stub"
	statusRoutedUnregistered = "routed-but-unregistered"
	statusMissing            = "missing"
)

// Effect is what a stock client observes.
const (
	effectOK          = "ok"
	effectEmpty       = "empty"
	effectBool        = "bool"
	effectNotModified = "not_modified"
	effectError       = "error"
)

// Site is one client call site.
type Site struct {
	Client string `json:"client"`
	File   string `json:"file"`
	Func   string `json:"func,omitempty"`
}

// MethodCalls collects call sites for one TL method.
type MethodCalls struct {
	Method string `json:"method"`
	Sites  []Site `json:"sites"`
}

// Source describes a client checkout the snapshot was extracted from.
type Source struct {
	Client string `json:"client"`
	Repo   string `json:"repo"`
	Commit string `json:"commit"`
	Date   string `json:"date,omitempty"`
	Note   string `json:"note"`
}

// Snapshot is the committed client-call inventory.
type Snapshot struct {
	Layer   int           `json:"layer"`
	Sources []Source      `json:"sources"`
	Methods []MethodCalls `json:"methods"`
}

// Row is one matrix record after server classification.
type Row struct {
	Method     string
	Module     string
	Phase      string
	When       string
	Startup    bool
	Background bool
	IOS        bool
	Android    bool
	Desktop    bool
	MacOS      bool
	Status     string
	Effect     string
	Route      bool
	Handler    string // implemented, stub, absent
	Service    string
	Notes      string
	Clients    map[string][]Site
}
