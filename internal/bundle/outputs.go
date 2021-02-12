package bundle

import (
	"fmt"
	"sort"

	"github.com/Jeffail/benthos/v3/internal/docs"
	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/output"
	"github.com/Jeffail/benthos/v3/lib/types"
)

// AllOutputs is a set containing every single output that has been imported.
var AllOutputs = &OutputSet{
	specs: map[string]outputSpec{},
}

//------------------------------------------------------------------------------

// OutputConstructor constructs an output component.
type OutputConstructor func(
	conf output.Config,
	mgr types.Manager,
	log log.Modular,
	stats metrics.Type,
	pipelines ...types.PipelineConstructorFunc,
) (types.Output, error)

type outputSpec struct {
	constructor OutputConstructor
	spec        docs.ComponentSpec
}

// OutputSet contains an explicit set of outputs available to a Benthos service.
type OutputSet struct {
	specs map[string]outputSpec
}

// Add a new output to this set by providing a spec (name, documentation, and
// constructor).
func (s *OutputSet) Add(constructor OutputConstructor, spec docs.ComponentSpec) error {
	if _, exists := s.specs[spec.Name]; exists {
		return fmt.Errorf("conflicting output name: %v", spec.Name)
	}
	s.specs[spec.Name] = outputSpec{
		constructor: constructor,
		spec:        spec,
	}
	return nil
}

// Init attempts to initialise an output from a config.
func (s *OutputSet) Init(
	conf output.Config,
	mgr types.Manager,
	log log.Modular,
	stats metrics.Type,
	pipelines ...types.PipelineConstructorFunc,
) (types.Output, error) {
	spec, exists := s.specs[conf.Type]
	if !exists {
		return nil, types.ErrInvalidOutputType
	}
	return spec.constructor(conf, mgr, log, stats, pipelines...)
}

// Docs returns a slice of output specs, which document each method.
func (s *OutputSet) Docs() []docs.ComponentSpec {
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
func (s *OutputSet) List() []string {
	names := make([]string, 0, len(s.specs))
	for k := range s.specs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
