package injectors

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// ExampleInjector is an example injector that demonstrates how to implement
// the port.Injector interface.
//
// To create a new injector:
//  1. Add the //docengine:injector comment before the struct
//  2. Implement the methods: Code(), Resolve(), IsCritical(), Timeout(),
//     DataType(), DefaultValue(), Formats()
//  3. Add translations in settings/injectors.i18n.yaml
//  4. Run: make gen
//
//docengine:injector
type ExampleInjector struct{}

// ExampleInjectorCode is the unique identifier of the injector.
// It is used to map with translations in injectors.i18n.yaml.
// It must be unique and immutable.
const injectorCode = "example_value"

// Code returns the unique identifier of the injector.
// This code is used to map with translations in injectors.i18n.yaml.
func (i *ExampleInjector) Code() string {
	return injectorCode
}

// Resolve returns the resolution function and the list of dependencies.
// - The function resolves and returns the injector's value.
// - Dependencies are codes of other injectors that must execute first.
func (i *ExampleInjector) Resolve() (port.ResolveFunc, []string) {
	return func(ctx context.Context, injCtx *entity.InjectorContext) (*entity.InjectorResult, error) {
		// Get initialization data (loaded in GlobalInit)
		// Note: Do not import extensions directly to avoid import cycles.
		// Use type assertion with the concrete type defined in extensions/init.go
		// initData := injCtx.InitData().(*extensions.InitializedData)
		_ = injCtx.InitData() // use as needed

		// Example: return a string value
		return &entity.InjectorResult{
			Value: entity.StringValue("example"),
		}, nil
	}, nil // no dependencies on other injectors
}

// IsCritical indicates if an error in this injector should stop the process.
// If true: the error stops document generation.
// If false: the error is logged and continues (the value remains empty).
func (i *ExampleInjector) IsCritical() bool {
	return false
}

// Timeout returns the timeout for this injector.
// If 0, the global default timeout (30s) is used.
func (i *ExampleInjector) Timeout() time.Duration {
	return 0
}

// DataType returns the type of value this injector produces.
// Used by frontend for display and validation.
func (i *ExampleInjector) DataType() entity.ValueType {
	return entity.ValueTypeString
}

// DefaultValue returns the default value if resolution fails.
// Return nil for no default (error will be raised if critical).
func (i *ExampleInjector) DefaultValue() *entity.InjectableValue {
	v := entity.StringValue("N/A")
	return &v
}

// Formats returns the format configuration for this injector.
// Return nil if formatting is not applicable.
func (i *ExampleInjector) Formats() *entity.FormatConfig {
	return nil // no formatting for plain strings
}
