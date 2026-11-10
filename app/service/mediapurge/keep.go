package mediapurge

import "strings"

// protectedObject is a key the purge worker must not delete. Crash-report
// copies use the crash-reports class. A document whose file_path was stored
// under that prefix is kept as well.
func protectedObject(class, path string) bool {
	class = strings.ToLower(strings.Trim(strings.TrimSpace(class), "/"))
	path = strings.ToLower(strings.Trim(strings.TrimSpace(path), "/"))
	if class == "crash-reports" {
		return true
	}
	return strings.HasPrefix(path, "crash-reports/")
}
