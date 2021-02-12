package manager

import (
	"context"
	"fmt"
	"net/http"
	"path"
	"sync"
	"time"

	"github.com/Jeffail/benthos/v3/internal/bundle"
	"github.com/Jeffail/benthos/v3/lib/cache"
	"github.com/Jeffail/benthos/v3/lib/condition"
	"github.com/Jeffail/benthos/v3/lib/input"
	"github.com/Jeffail/benthos/v3/lib/log"
	"github.com/Jeffail/benthos/v3/lib/metrics"
	"github.com/Jeffail/benthos/v3/lib/output"
	"github.com/Jeffail/benthos/v3/lib/processor"
	"github.com/Jeffail/benthos/v3/lib/ratelimit"
	"github.com/Jeffail/benthos/v3/lib/types"
)

// ErrResourceNotFound represents an error where a named resource could not be
// accessed because it was not found by the manager.
type ErrResourceNotFound string

// Error implements the standard error interface.
func (e ErrResourceNotFound) Error() string {
	return fmt.Sprintf("unable to locate resource: %v", string(e))
}

//------------------------------------------------------------------------------

// APIReg is an interface representing an API builder.
type APIReg interface {
	RegisterEndpoint(path, desc string, h http.HandlerFunc)
}

//------------------------------------------------------------------------------

// Type is an implementation of types.Manager, which is expected by Benthos
// components that need to register service wide behaviours such as HTTP
// endpoints and event listeners, and obtain service wide shared resources such
// as caches and other resources.
type Type struct {
	// An optional identifier given to a manager that is used by a unique stream
	// and if specified should be used as a path prefix for API endpoints, and
	// added as a label to logs and metrics.
	stream string

	// An optional identifier given to a manager that is used by a component and
	// if specified should be added as a label to logs and metrics.
	component string

	apiReg APIReg

	inputs       map[string]types.Input
	caches       map[string]types.Cache
	processors   map[string]types.Processor
	outputs      map[string]types.OutputWriter
	rateLimits   map[string]types.RateLimit
	plugins      map[string]interface{}
	resourceLock *sync.RWMutex

	// Collections of component constructors
	inputBundle *bundle.InputSet

	logger log.Modular
	stats  metrics.Type

	pipes    map[string]<-chan types.Transaction
	pipeLock *sync.RWMutex

	// TODO: V4 Remove this
	conditions map[string]types.Condition
}

// New returns an instance of manager.Type, which can be shared amongst
// components and logical threads of a Benthos service.
func New(
	conf Config,
	apiReg APIReg,
	log log.Modular,
	stats metrics.Type,
) (*Type, error) {
	t := &Type{
		apiReg: apiReg,

		inputs:       map[string]types.Input{},
		caches:       map[string]types.Cache{},
		processors:   map[string]types.Processor{},
		outputs:      map[string]types.OutputWriter{},
		rateLimits:   map[string]types.RateLimit{},
		plugins:      map[string]interface{}{},
		resourceLock: &sync.RWMutex{},

		// All bundles default to everything that was imported.
		inputBundle: bundle.AllInputs,

		logger: log,
		stats:  stats,

		pipes:    map[string]<-chan types.Transaction{},
		pipeLock: &sync.RWMutex{},

		conditions: map[string]types.Condition{},
	}

	// Sometimes resources of a type might refer to other resources of the same
	// type. When they are constructed they will check with the manager to
	// ensure the resource they point to is valid, but not keep the reference.
	// Since we cannot guarantee an order of initialisation we create
	// placeholders during construction.
	for k := range conf.Inputs {
		t.inputs[k] = nil
	}
	for k := range conf.Caches {
		t.caches[k] = nil
	}
	for k := range conf.Conditions {
		t.conditions[k] = nil
	}
	for k := range conf.Processors {
		t.processors[k] = nil
	}
	for k := range conf.Outputs {
		t.outputs[k] = nil
	}
	for k := range conf.RateLimits {
		t.rateLimits[k] = nil
	}
	for k, conf := range conf.Plugins {
		if _, exists := pluginSpecs[conf.Type]; !exists {
			continue
		}
		t.plugins[k] = nil
	}

	for k, conf := range conf.Inputs {
		if err := t.StoreInput(context.Background(), k, conf); err != nil {
			return nil, err
		}
	}

	for k, conf := range conf.Caches {
		newCache, err := cache.New(conf, t, log.NewModule(".resource.cache."+k), metrics.Namespaced(stats, "resource.cache."+k))
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create cache resource '%v' of type '%v': %v",
				k, conf.Type, err,
			)
		}
		t.caches[k] = newCache
	}

	// TODO: Prevent recursive conditions.
	for k, newConf := range conf.Conditions {
		newCond, err := condition.New(newConf, t, log.NewModule(".resource.condition."+k), metrics.Namespaced(stats, "resource.condition."+k))
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create condition resource '%v' of type '%v': %v",
				k, newConf.Type, err,
			)
		}

		t.conditions[k] = newCond
	}

	// TODO: Prevent recursive processors.
	for k, newConf := range conf.Processors {
		newProc, err := processor.New(newConf, t, log.NewModule(".resource.processor."+k), metrics.Namespaced(stats, "resource.processor."+k))
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create processor resource '%v' of type '%v': %v",
				k, newConf.Type, err,
			)
		}

		t.processors[k] = newProc
	}

	for k, conf := range conf.RateLimits {
		newRL, err := ratelimit.New(conf, t, log.NewModule(".resource.rate_limit."+k), metrics.Namespaced(stats, "resource.rate_limit."+k))
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create rate_limit resource '%v' of type '%v': %v",
				k, conf.Type, err,
			)
		}
		t.rateLimits[k] = newRL
	}

	for k, conf := range conf.Outputs {
		newOutput, err := output.New(conf, t, log.NewModule(".resource.output."+k), metrics.Namespaced(stats, "resource.output."+k))
		if err == nil {
			t.outputs[k], err = wrapOutput(newOutput)
		}
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create output resource '%v' of type '%v': %v",
				k, conf.Type, err,
			)
		}
	}

	for k, conf := range conf.Plugins {
		spec, exists := pluginSpecs[conf.Type]
		if !exists {
			return nil, fmt.Errorf("unrecognised plugin type '%v'", conf.Type)
		}
		newP, err := spec.constructor(conf.Plugin, t, log.NewModule(".resource.plugin."+k), metrics.Namespaced(stats, "resource.plugin."+k))
		if err != nil {
			return nil, fmt.Errorf(
				"failed to create plugin resource '%v' of type '%v': %v",
				k, conf.Type, err,
			)
		}
		t.plugins[k] = newP
	}

	return t, nil
}

//------------------------------------------------------------------------------

func unwrapMetric(t metrics.Type) metrics.Type {
	u, ok := t.(interface {
		Unwrap() metrics.Type
	})
	if ok {
		t = u.Unwrap()
	}
	return t
}

// ForStream returns a variant of this manager to be used by a particular stream
// identifer, where APIs registered will be namespaced by that id.
func (t *Type) ForStream(id string) types.Manager {
	return t.forStream(id)
}

func (t *Type) forStream(id string) *Type {
	newT := *t
	newT.stream = id
	newT.logger = t.logger.WithFields(map[string]string{
		"stream": id,
	})
	newT.stats = metrics.Namespaced(unwrapMetric(t.stats), id)
	return &newT
}

// ForComponent returns a variant of this manager to be used by a particular
// component identifer, where observability components will be automatically
// tagged with the label.
func (t *Type) ForComponent(id string) types.Manager {
	return t.forComponent(id)
}

func (t *Type) forComponent(id string) *Type {
	newT := *t
	newT.component = id
	newT.logger = t.logger.WithFields(map[string]string{
		"component": id,
	})

	statsPrefix := id
	if len(newT.stream) > 0 {
		statsPrefix = newT.stream + "." + statsPrefix
	}
	newT.stats = metrics.Namespaced(unwrapMetric(t.stats), statsPrefix)
	return &newT
}

// ForChildComponent returns a variant of this manager to be used by a
// particular component identifer, which is a child of the current component,
// where observability components will be automatically tagged with the label.
func (t *Type) ForChildComponent(id string) types.Manager {
	return t.forChildComponent(id)
}

func (t *Type) forChildComponent(id string) *Type {
	newT := *t
	newT.logger = t.logger.NewModule("." + id)
	newT.stats = metrics.Namespaced(t.stats, id)

	if len(newT.component) > 0 {
		id = newT.component + "." + id
	}
	newT.component = id
	return &newT
}

// Label returns the current component label held by a manager.
func (t *Type) Label() string {
	return t.component
}

//------------------------------------------------------------------------------

// RegisterEndpoint registers a server wide HTTP endpoint.
func (t *Type) RegisterEndpoint(apiPath, desc string, h http.HandlerFunc) {
	if len(t.stream) > 0 {
		apiPath = path.Join("/", t.stream, apiPath)
	}
	t.apiReg.RegisterEndpoint(apiPath, desc, h)
}

// SetPipe registers a new transaction chan to a named pipe.
func (t *Type) SetPipe(name string, tran <-chan types.Transaction) {
	t.pipeLock.Lock()
	t.pipes[name] = tran
	t.pipeLock.Unlock()
}

// GetPipe attempts to obtain and return a named output Pipe
func (t *Type) GetPipe(name string) (<-chan types.Transaction, error) {
	t.pipeLock.RLock()
	pipe, exists := t.pipes[name]
	t.pipeLock.RUnlock()
	if exists {
		return pipe, nil
	}
	return nil, types.ErrPipeNotFound
}

// UnsetPipe removes a named pipe transaction chan.
func (t *Type) UnsetPipe(name string, tran <-chan types.Transaction) {
	t.pipeLock.Lock()
	if otran, exists := t.pipes[name]; exists && otran == tran {
		delete(t.pipes, name)
	}
	t.pipeLock.Unlock()
}

//------------------------------------------------------------------------------

// Metrics returns an aggregator preset with the current component context.
func (t *Type) Metrics() metrics.Type {
	return t.stats
}

// Logger returns a logger preset with the current component context.
func (t *Type) Logger() log.Modular {
	return t.logger
}

//------------------------------------------------------------------------------

func closeWithContext(ctx context.Context, c types.Closable) error {
	c.CloseAsync()
	waitFor := time.Second
	deadline, hasDeadline := ctx.Deadline()
	if hasDeadline {
		waitFor = time.Until(deadline)
	}
	err := c.WaitForClose(waitFor)
	for err != nil && !hasDeadline {
		err = c.WaitForClose(time.Second)
	}
	return err
}

// AccessInput attempts to access an input resource by a unique identifier and
// executes a closure function with the input as an argument. Returns an error
// if the input does not exist (or is otherwise inaccessible).
//
// During the execution of the provided closure it is guaranteed that the
// resource will not be closed or removed. However, it is possible for the
// resource to be accessed by any number of components in parallel.
func (t *Type) AccessInput(ctx context.Context, name string, fn func(i types.Input)) error {
	// TODO: Eventually use ctx to cancel blocking on the mutex lock. Needs
	// profiling for heavy use within a busy loop.
	t.resourceLock.RLock()
	defer t.resourceLock.RUnlock()
	i, ok := t.inputs[name]
	if !ok {
		return ErrResourceNotFound(name)
	}
	fn(i)
	return nil
}

// NewInput attempts to create a new input component from a config.
//
// TODO: V4 Remove the dumb batch field.
func (t *Type) NewInput(conf input.Config, hasBatchProc bool) (types.Input, error) {
	mgr := t
	/*
		// A configured label overrides any previously set component label.
		if len(conf.Label) > 0 && t.component != conf.Label {
			mgr = t.ForComponent(conf.Label)
		}
	*/
	// TODO: Check whether we're using a custom component set
	return t.inputBundle.Init(hasBatchProc, conf, mgr, mgr.Logger(), mgr.Metrics())
}

// StoreInput attempts to store a new input resource. If an existing resource
// has the same name it is closed and removed _before_ the new one is
// initialized in order to avoid duplicate connections.
func (t *Type) StoreInput(ctx context.Context, name string, conf input.Config) error {
	t.resourceLock.Lock()
	defer t.resourceLock.Unlock()

	i, ok := t.inputs[name]
	if ok {
		// If a previous resource exists with the same name then we do NOT allow
		// it to be replaced unless it can be successfully closed. This ensures
		// that we do not leak connections.
		if err := closeWithContext(ctx, i); err != nil {
			return err
		}
	}

	newInput, err := t.forComponent("resource.input."+name).NewInput(conf, false)
	if err != nil {
		return fmt.Errorf(
			"failed to create input resource '%v' of type '%v': %w",
			name, conf.Type, err,
		)
	}

	t.inputs[name] = newInput
	return nil
}

//------------------------------------------------------------------------------

// CloseAsync triggers the shut down of all resource types that implement the
// lifetime interface types.Closable.
func (t *Type) CloseAsync() {
	t.resourceLock.Lock()
	defer t.resourceLock.Unlock()

	for _, c := range t.inputs {
		c.CloseAsync()
	}
	for _, c := range t.caches {
		c.CloseAsync()
	}
	for _, c := range t.conditions {
		if closer, ok := c.(types.Closable); ok {
			closer.CloseAsync()
		}
	}
	for _, p := range t.processors {
		p.CloseAsync()
	}
	for _, c := range t.plugins {
		if closer, ok := c.(types.Closable); ok {
			closer.CloseAsync()
		}
	}
	for _, c := range t.rateLimits {
		c.CloseAsync()
	}
	for _, c := range t.outputs {
		c.CloseAsync()
	}
}

// WaitForClose blocks until either all closable resource types are shut down or
// a timeout occurs.
func (t *Type) WaitForClose(timeout time.Duration) error {
	t.resourceLock.Lock()
	defer t.resourceLock.Unlock()

	timesOut := time.Now().Add(timeout)
	for k, c := range t.inputs {
		if err := c.WaitForClose(time.Until(timesOut)); err != nil {
			return fmt.Errorf("resource '%s' failed to cleanly shutdown: %v", k, err)
		}
		delete(t.inputs, k)
	}
	for k, c := range t.caches {
		if err := c.WaitForClose(time.Until(timesOut)); err != nil {
			return fmt.Errorf("resource '%s' failed to cleanly shutdown: %v", k, err)
		}
		delete(t.caches, k)
	}
	for k, p := range t.processors {
		if err := p.WaitForClose(time.Until(timesOut)); err != nil {
			return fmt.Errorf("resource '%s' failed to cleanly shutdown: %v", k, err)
		}
		delete(t.processors, k)
	}
	for k, c := range t.rateLimits {
		if err := c.WaitForClose(time.Until(timesOut)); err != nil {
			return fmt.Errorf("resource '%s' failed to cleanly shutdown: %v", k, err)
		}
		delete(t.rateLimits, k)
	}
	for k, c := range t.outputs {
		if err := c.WaitForClose(time.Until(timesOut)); err != nil {
			return fmt.Errorf("resource '%s' failed to cleanly shutdown: %v", k, err)
		}
		delete(t.outputs, k)
	}
	for k, c := range t.plugins {
		if closer, ok := c.(types.Closable); ok {
			if err := closer.WaitForClose(time.Until(timesOut)); err != nil {
				return fmt.Errorf("resource '%s' failed to cleanly shutdown: %v", k, err)
			}
		}
		delete(t.plugins, k)
	}
	return nil
}

//------------------------------------------------------------------------------

// DEPRECATED
// TODO: V4 Remove this

// GetInput attempts to find a service wide input by its name.
func (t *Type) GetInput(name string) (types.Input, error) {
	if c, exists := t.inputs[name]; exists {
		return c, nil
	}
	return nil, types.ErrInputNotFound
}

// GetCache attempts to find a service wide cache by its name.
func (t *Type) GetCache(name string) (types.Cache, error) {
	if c, exists := t.caches[name]; exists {
		return c, nil
	}
	return nil, types.ErrCacheNotFound
}

// GetCondition attempts to find a service wide condition by its name.
func (t *Type) GetCondition(name string) (types.Condition, error) {
	if c, exists := t.conditions[name]; exists {
		return c, nil
	}
	return nil, types.ErrConditionNotFound
}

// GetProcessor attempts to find a service wide processor by its name.
func (t *Type) GetProcessor(name string) (types.Processor, error) {
	if p, exists := t.processors[name]; exists {
		return p, nil
	}
	return nil, types.ErrProcessorNotFound
}

// GetRateLimit attempts to find a service wide rate limit by its name.
func (t *Type) GetRateLimit(name string) (types.RateLimit, error) {
	if rl, exists := t.rateLimits[name]; exists {
		return rl, nil
	}
	return nil, types.ErrRateLimitNotFound
}

// GetOutput attempts to find a service wide output by its name.
func (t *Type) GetOutput(name string) (types.OutputWriter, error) {
	if c, exists := t.outputs[name]; exists {
		return c, nil
	}
	return nil, types.ErrOutputNotFound
}

// GetPlugin attempts to find a service wide resource plugin by its name.
func (t *Type) GetPlugin(name string) (interface{}, error) {
	if pl, exists := t.plugins[name]; exists {
		return pl, nil
	}
	return nil, types.ErrPluginNotFound
}
