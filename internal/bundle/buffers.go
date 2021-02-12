package bundle

import (
	"fmt"
	"sort"

	"github.com/Jeffail/benthos/v3/internal/docs"
	"github.com/Jeffail/benthos/v3/lib/buffer"
	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/types"
)

// AllBuffers is a set containing every single buffer that has been imported.
var AllBuffers = &BufferSet{
	specs: map[string]bufferSpec{},
}

//------------------------------------------------------------------------------

// BufferConstructor constructs an buffer component.
type BufferConstructor func(
	conf buffer.Config,
	mgr types.Manager,
	log log.Modular,
	stats metrics.Type,
) (buffer.Type, error)

type bufferSpec struct {
	constructor BufferConstructor
	spec        docs.ComponentSpec
}

// BufferSet contains an explicit set of buffers available to a Benthos service.
type BufferSet struct {
	specs map[string]bufferSpec
}

// Add a new buffer to this set by providing a spec (name, documentation, and
// constructor).
func (s *BufferSet) Add(constructor BufferConstructor, spec docs.ComponentSpec) error {
	if _, exists := s.specs[spec.Name]; exists {
		return fmt.Errorf("conflicting buffer name: %v", spec.Name)
	}
	s.specs[spec.Name] = bufferSpec{
		constructor: constructor,
		spec:        spec,
	}
	return nil
}

// Init attempts to initialise an buffer from a config.
func (s *BufferSet) Init(
	conf buffer.Config,
	mgr types.Manager,
	log log.Modular,
	stats metrics.Type,
) (buffer.Type, error) {
	spec, exists := s.specs[conf.Type]
	if !exists {
		return nil, types.ErrInvalidBufferType
	}
	return spec.constructor(conf, mgr, log, stats)
}

// Docs returns a slice of buffer specs, which document each method.
func (s *BufferSet) Docs() []docs.ComponentSpec {
	var docs []docs.ComponentSpec
	for _, v := range s.specs {
		docs = append(docs, v.spec)
	}
	sort.Slice(docs, func(i, j int) bool {
		return docs[i].Name < docs[j].Name
	})
	return docs
}

// List returns a slice of method names in alphabetical order.
func (s *BufferSet) List() []string {
	names := make([]string, 0, len(s.specs))
	for k := range s.specs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
