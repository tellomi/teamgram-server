package model

import (
	"path"
	"strings"
)

// AnimatedFileName is the document filename for an animated GIF.
// A name that is already .mp4 is kept. Other extensions become .mp4 once,
// so "x.mp4" is not rewritten to "x.mp4.mp4" and is not used as a DFS path.
func AnimatedFileName(name string) string {
	name = path.Base(strings.TrimSpace(name))
	if name == "" || name == "." || name == "/" {
		return "animation.mp4"
	}
	ext := strings.ToLower(path.Ext(name))
	if ext == ".mp4" {
		return name
	}
	base := strings.TrimSuffix(name, path.Ext(name))
	if base == "" {
		base = "animation"
	}
	return base + ".mp4"
}
