package loader

import (
	"fmt"
	"sync"

	"srdashboard/host/logicapi"
)

var (
	builtinMu sync.RWMutex
	builtins  = map[string]func(manifest *Manifest) logicapi.Logic{}
)

// RegisterBuiltin registers an in-process Logic factory for a plugin id.
func RegisterBuiltin(id string, factory func(manifest *Manifest) logicapi.Logic) {
	builtinMu.Lock()
	defer builtinMu.Unlock()
	builtins[id] = factory
}

func lookupBuiltin(id string) (func(manifest *Manifest) logicapi.Logic, bool) {
	builtinMu.RLock()
	defer builtinMu.RUnlock()
	f, ok := builtins[id]
	return f, ok
}

func newBuiltinLogic(manifest *Manifest) (logicapi.Logic, error) {
	f, ok := lookupBuiltin(manifest.ID)
	if !ok {
		return nil, fmt.Errorf("no builtin logic registered for plugin %q", manifest.ID)
	}
	return f(manifest), nil
}
