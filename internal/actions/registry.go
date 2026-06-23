package actions

import "fmt"

type Registry struct {
	specs map[string]Spec
}

func NewRegistry(specs []Spec) (*Registry, error) {
	r := &Registry{specs: make(map[string]Spec, len(specs))}
	for _, spec := range specs {
		if spec.Name == "" {
			return nil, ErrInvalidSpec
		}
		if spec.BuildArgv == nil {
			return nil, fmt.Errorf("%w: action %q has no argv builder", ErrInvalidSpec, spec.Name)
		}
		if _, exists := r.specs[spec.Name]; exists {
			return nil, fmt.Errorf("%w: duplicate action %q", ErrInvalidSpec, spec.Name)
		}
		if spec.SensitiveArgs == nil {
			spec.SensitiveArgs = make(map[string]struct{})
		}
		for _, arg := range spec.Args {
			if arg.Name == "" || arg.Type == "" {
				return nil, fmt.Errorf("%w: action %q has invalid argument spec", ErrInvalidSpec, spec.Name)
			}
			if arg.Sensitive {
				spec.SensitiveArgs[arg.Name] = struct{}{}
			}
		}
		r.specs[spec.Name] = spec
	}
	return r, nil
}

func NewDefaultRegistry() (*Registry, error) {
	return NewRegistry(DefaultSpecs())
}

func (r *Registry) Get(name string) (Spec, bool) {
	if r == nil {
		return Spec{}, false
	}
	spec, ok := r.specs[name]
	return spec, ok
}

func (r *Registry) Validate(name string, args map[string]any) (Spec, ValidatedArgs, error) {
	spec, ok := r.Get(name)
	if !ok {
		return Spec{}, nil, fmt.Errorf("unknown action %q", name)
	}
	validated, err := ValidateArgs(spec, args)
	if err != nil {
		return Spec{}, nil, err
	}
	return spec, validated, nil
}

func (r *Registry) ValidateAllowedActions(allowed []string) error {
	if len(allowed) == 0 {
		return fmt.Errorf("allowed actions must not be empty")
	}
	for _, action := range allowed {
		if _, ok := r.Get(action); !ok {
			return fmt.Errorf("unknown allowed action %q", action)
		}
	}
	return nil
}

func (r *Registry) Names() []string {
	names := make([]string, 0, len(r.specs))
	for name := range r.specs {
		names = append(names, name)
	}
	return names
}
