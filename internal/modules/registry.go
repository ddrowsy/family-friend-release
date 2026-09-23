package modules

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/ddrowsy/family-friend-release/internal/config"
)

// Registry stores available modules by name and tracks started modules.
type Registry struct {
	modules map[string]Module
	started []Module
}

// StartResult describes the outcome of applying a policy to registered modules.
type StartResult struct {
	Started []string
	Skipped []string
	Unknown []string
}

// NewRegistry creates an empty module registry.
func NewRegistry() *Registry {
	return &Registry{modules: make(map[string]Module)}
}

// Register adds a module to the registry.
func (r *Registry) Register(module Module) error {
	if module == nil {
		return errors.New("module is nil")
	}

	name := module.Name()
	if name == "" {
		return errors.New("module name is required")
	}
	if _, exists := r.modules[name]; exists {
		return fmt.Errorf("module %q is already registered", name)
	}

	r.modules[name] = module
	return nil
}

// Get returns a registered module.
func (r *Registry) Get(name string) (Module, bool) {
	module, ok := r.modules[name]
	return module, ok
}

// Names returns registered module names in sorted order.
func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.modules))
	for name := range r.modules {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// StartEnabled starts registered modules that are enabled in policy.
func (r *Registry) StartEnabled(ctx context.Context, policy config.Policy) (StartResult, error) {
	result := StartResult{}

	moduleNames := make([]string, 0, len(policy.Modules))
	for name := range policy.Modules {
		moduleNames = append(moduleNames, name)
	}
	sort.Strings(moduleNames)

	for _, name := range moduleNames {
		modulePolicy := policy.Modules[name]
		module, ok := r.modules[name]
		if !ok {
			result.Unknown = append(result.Unknown, name)
			continue
		}
		if !modulePolicy.Enabled {
			result.Skipped = append(result.Skipped, name)
			continue
		}

		if err := module.Start(ctx, modulePolicy); err != nil {
			return result, fmt.Errorf("start module %q: %w", name, err)
		}

		r.started = append(r.started, module)
		result.Started = append(result.Started, name)
	}

	return result, nil
}

// StopStarted stops modules that were started by this registry.
func (r *Registry) StopStarted(ctx context.Context) error {
	var err error

	for i := len(r.started) - 1; i >= 0; i-- {
		module := r.started[i]
		if stopErr := module.Stop(ctx); stopErr != nil {
			err = errors.Join(err, fmt.Errorf("stop module %q: %w", module.Name(), stopErr))
		}
	}

	r.started = nil
	return err
}
