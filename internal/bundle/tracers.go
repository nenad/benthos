package bundle

import (
	"fmt"
	"sort"

	"github.com/Jeffail/benthos/v3/internal/docs"
	"github.com/Jeffail/benthos/v3/lib/tracer"
)

// AllTracers is a set containing every single tracer that has been imported.
var AllTracers = &TracerSet{
	specs: map[string]tracerSpec{},
}

//------------------------------------------------------------------------------

// TracerConstructor constructs an tracer component.
type TracerConstructor func(conf tracer.Config, opts ...func(tracer.Type)) (tracer.Type, error)

type tracerSpec struct {
	constructor TracerConstructor
	spec        docs.ComponentSpec
}

// TracerSet contains an explicit set of tracers available to a Benthos service.
type TracerSet struct {
	specs map[string]tracerSpec
}

// Add a new tracer to this set by providing a spec (name, documentation, and
// constructor).
func (s *TracerSet) Add(constructor TracerConstructor, spec docs.ComponentSpec) error {
	if _, exists := s.specs[spec.Name]; exists {
		return fmt.Errorf("conflicting tracer name: %v", spec.Name)
	}
	s.specs[spec.Name] = tracerSpec{
		constructor: constructor,
		spec:        spec,
	}
	return nil
}

// Init attempts to initialise an tracer from a config.
func (s *TracerSet) Init(conf tracer.Config, opts ...func(tracer.Type)) (tracer.Type, error) {
	spec, exists := s.specs[conf.Type]
	if !exists {
		return nil, tracer.ErrInvalidTracerType
	}
	return spec.constructor(conf, opts...)
}

// Docs returns a slice of tracer specs, which document each method.
func (s *TracerSet) Docs() []docs.ComponentSpec {
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
func (s *TracerSet) List() []string {
	names := make([]string, 0, len(s.specs))
	for k := range s.specs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
