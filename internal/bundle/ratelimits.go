package bundle

import (
	"fmt"
	"sort"

	"github.com/Jeffail/benthos/v3/internal/docs"
	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/ratelimit"
	"github.com/Jeffail/benthos/v3/lib/types"
)

// AllRateLimits is a set containing every single ratelimit that has been imported.
var AllRateLimits = &RateLimitSet{
	specs: map[string]rateLimitSpec{},
}

//------------------------------------------------------------------------------

// RateLimitConstructor constructs an ratelimit component.
type RateLimitConstructor func(
	conf ratelimit.Config,
	mgr types.Manager,
	log log.Modular,
	stats metrics.Type,
) (types.RateLimit, error)

type rateLimitSpec struct {
	constructor RateLimitConstructor
	spec        docs.ComponentSpec
}

// RateLimitSet contains an explicit set of ratelimits available to a Benthos service.
type RateLimitSet struct {
	specs map[string]rateLimitSpec
}

// Add a new ratelimit to this set by providing a spec (name, documentation, and
// constructor).
func (s *RateLimitSet) Add(constructor RateLimitConstructor, spec docs.ComponentSpec) error {
	if _, exists := s.specs[spec.Name]; exists {
		return fmt.Errorf("conflicting ratelimit name: %v", spec.Name)
	}
	s.specs[spec.Name] = rateLimitSpec{
		constructor: constructor,
		spec:        spec,
	}
	return nil
}

// Init attempts to initialise an ratelimit from a config.
func (s *RateLimitSet) Init(
	conf ratelimit.Config,
	mgr types.Manager,
	log log.Modular,
	stats metrics.Type,
) (types.RateLimit, error) {
	spec, exists := s.specs[conf.Type]
	if !exists {
		return nil, types.ErrInvalidRateLimitType
	}
	return spec.constructor(conf, mgr, log, stats)
}

// Docs returns a slice of ratelimit specs, which document each method.
func (s *RateLimitSet) Docs() []docs.ComponentSpec {
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
func (s *RateLimitSet) List() []string {
	names := make([]string, 0, len(s.specs))
	for k := range s.specs {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
