package injectors

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// ExampleImageInjector is an example injector that returns a sample image URL.
// This demonstrates how to implement an IMAGE type injectable.
//
// IMAGE injectables return string URLs that are resolved when rendering documents.
// The frontend displays the image using the resolved URL.
//
//docengine:injector
type ExampleImageInjector struct{}

const exampleImageCode = "example_image"

// Code returns the unique identifier of the injector.
func (i *ExampleImageInjector) Code() string {
	return exampleImageCode
}

// Resolve returns the resolution function and the list of dependencies.
func (i *ExampleImageInjector) Resolve() (port.ResolveFunc, []string) {
	return func(ctx context.Context, injCtx *entity.InjectorContext) (*entity.InjectorResult, error) {
		// Return a sample image URL from picsum.photos
		// In a real implementation, this could fetch from an API, database, or external service
		return &entity.InjectorResult{
			Value: entity.ImageValue("https://picsum.photos/seed/example/400/300"),
		}, nil
	}, nil // no dependencies
}

// IsCritical indicates if an error in this injector should stop the process.
func (i *ExampleImageInjector) IsCritical() bool {
	return false
}

// Timeout returns the timeout for this injector.
func (i *ExampleImageInjector) Timeout() time.Duration {
	return 0 // use default
}

// DataType returns the type of value this injector produces.
func (i *ExampleImageInjector) DataType() entity.ValueType {
	return entity.ValueTypeImage
}

// DefaultValue returns the default value if resolution fails.
func (i *ExampleImageInjector) DefaultValue() *entity.InjectableValue {
	v := entity.ImageValue("https://picsum.photos/400/300")
	return &v
}

// Formats returns the format configuration for this injector.
func (i *ExampleImageInjector) Formats() *entity.FormatConfig {
	return nil // no formatting for image URLs
}
