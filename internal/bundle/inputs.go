package bundle

import (
	"fmt"
	"sort"

	"github.com/Jeffail/benthos/v3/internal/docs"
	"github.com/Jeffail/benthos/v3/lib/input"
	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/types"
)

// AllInputs is a set containing every single input that has been imported.
var AllInputs = &InputSet{
	specs: map[string]inputSpec{},
}

//------------------------------------------------------------------------------

// InputConstructor constructs an input component.
type InputConstructor input.ConstructorFunc

type inputSpec struct {
	constructor InputConstructor
	spec        docs.ComponentSpec
}

// InputSet contains an explicit set of inputs available to a Benthos service.
type InputSet struct {
	specs map[string]inputSpec
}

// Add a new input to this set by providing a spec (name, documentation, and
// constructor).
func (s *InputSet) Add(constructor InputConstructor, spec docs.ComponentSpec) error {
	if _, exists := s.specs[spec.Name]; exists {
		return fmt.Errorf("conflicting input name: %v", spec.Name)
	}
	s.specs[spec.Name] = inputSpec{
		constructor: constructor,
		spec:        spec,
	}
	return nil
}

// Init attempts to initialise an input from a config.
func (s *InputSet) Init(
	hasBatchProc bool,
	conf input.Config,
	mgr types.Manager,
	log log.Modular,
	stats metrics.Type,
	pipelines ...types.PipelineConstructorFunc,
) (types.Input, error) {
	spec, exists := s.specs[conf.Type]
	if !exists {
		return nil, types.ErrInvalidInputType
	}
	return spec.constructor(hasBatchProc, conf, mgr, log, stats, pipelines...)
}

// Docs returns a slice of input specs, which document each method.
func (s *InputSet) Docs() []docs.ComponentSpec {
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
func (s *InputSet) List() []string {
	names := make([]string, 0, len(s.specs))
	for k := range s.specs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
