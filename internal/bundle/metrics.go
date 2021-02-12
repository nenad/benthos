package bundle

import (
	"fmt"
	"sort"

	"github.com/Jeffail/benthos/v3/internal/docs"
	"github.com/Jeffail/benthos/v3/lib/metrics"
)

// AllMetrics is a set containing every single metrics that has been imported.
var AllMetrics = &MetricsSet{
	specs: map[string]metricsSpec{},
}

//------------------------------------------------------------------------------

// MetricConstructor constructs an metrics component.
type MetricConstructor func(conf metrics.Config, opts ...func(metrics.Type)) (metrics.Type, error)

type metricsSpec struct {
	constructor MetricConstructor
	spec        docs.ComponentSpec
}

// MetricsSet contains an explicit set of metrics available to a Benthos
// service.
type MetricsSet struct {
	specs map[string]metricsSpec
}

// Add a new metrics to this set by providing a spec (name, documentation, and
// constructor).
func (s *MetricsSet) Add(constructor MetricConstructor, spec docs.ComponentSpec) error {
	if _, exists := s.specs[spec.Name]; exists {
		return fmt.Errorf("conflicting metrics name: %v", spec.Name)
	}
	s.specs[spec.Name] = metricsSpec{
		constructor: constructor,
		spec:        spec,
	}
	return nil
}

// Init attempts to initialise an metrics from a config.
func (s *MetricsSet) Init(conf metrics.Config, opts ...func(metrics.Type)) (metrics.Type, error) {
	spec, exists := s.specs[conf.Type]
	if !exists {
		return nil, metrics.ErrInvalidMetricOutputType
	}
	return spec.constructor(conf, opts...)
}

// Docs returns a slice of metrics specs, which document each method.
func (s *MetricsSet) Docs() []docs.ComponentSpec {
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
func (s *MetricsSet) List() []string {
	names := make([]string, 0, len(s.specs))
	for k := range s.specs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
