package bundle

import (
	"fmt"
	"sort"

	"github.com/Jeffail/benthos/v3/internal/docs"
	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/processor"
	"github.com/Jeffail/benthos/v3/lib/types"
)

// AllProcessors is a set containing every single processor that has been
// imported.
var AllProcessors = &ProcessorSet{
	specs: map[string]processorSpec{},
}

//------------------------------------------------------------------------------

// ProcessorConstructor constructs an processor component.
type ProcessorConstructor func(
	conf processor.Config,
	mgr types.Manager,
	log log.Modular,
	stats metrics.Type,
) (types.Processor, error)

type processorSpec struct {
	constructor ProcessorConstructor
	spec        docs.ComponentSpec
}

// ProcessorSet contains an explicit set of processors available to a Benthos
// service.
type ProcessorSet struct {
	specs map[string]processorSpec
}

// Add a new processor to this set by providing a spec (name, documentation, and
// constructor).
func (s *ProcessorSet) Add(constructor ProcessorConstructor, spec docs.ComponentSpec) error {
	if _, exists := s.specs[spec.Name]; exists {
		return fmt.Errorf("conflicting processor name: %v", spec.Name)
	}
	s.specs[spec.Name] = processorSpec{
		constructor: constructor,
		spec:        spec,
	}
	return nil
}

// Init attempts to initialise an processor from a config.
func (s *ProcessorSet) Init(
	conf processor.Config,
	mgr types.Manager,
	log log.Modular,
	stats metrics.Type,
) (types.Processor, error) {
	spec, exists := s.specs[conf.Type]
	if !exists {
		return nil, types.ErrInvalidProcessorType
	}
	return spec.constructor(conf, mgr, log, stats)
}

// Docs returns a slice of processor specs, which document each method.
func (s *ProcessorSet) Docs() []docs.ComponentSpec {
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
func (s *ProcessorSet) List() []string {
	names := make([]string, 0, len(s.specs))
	for k := range s.specs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
