// Package all is a bundle that, when imported, imports all connectors that ship
// with the open source Benthos repo.
package all

import (
	"github.com/Jeffail/benthos/v3/internal/bundle"
	"github.com/Jeffail/benthos/v3/internal/docs"
	"github.com/Jeffail/benthos/v3/lib/input"
)

func init() {
	input.WalkConstructors(func(ctor input.ConstructorFunc, spec docs.ComponentSpec) {
		bundle.AllInputs.Add(bundle.InputConstructor(ctor), spec)
	})
}
