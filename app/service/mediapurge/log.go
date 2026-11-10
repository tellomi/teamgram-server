package mediapurge

import "log"

// logger is replaced by tests. It never receives message text or secrets.
var logger interface {
	Printf(string, ...any)
} = log.Default()
