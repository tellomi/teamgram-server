package usernamepolicy

import (
	_ "embed"
	"sync"
)

//go:embed policy/policy-2026110301.json
var embeddedPolicy []byte

var (
	loadOnce sync.Once
	loaded   *Engine
	loadErr  error
)

func embeddedEngine() *Engine {
	loadOnce.Do(func() {
		loaded, loadErr = Load(embeddedPolicy)
	})
	if loadErr != nil {
		panic(loadErr)
	}
	return loaded
}
