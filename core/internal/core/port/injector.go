package port

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// ResolveFunc is the function that resolves the injector value.
type ResolveFunc func(ctx context.Context, injCtx *entity.InjectorContext) (*entity.InjectorResult, error)

// Injector defines the interface that users implement.
type Injector interface {
	// Code returns the unique identifier of the injector.
	// It maps with injectors.i18n.yaml to get name and description.
	Code() string

	// Resolve returns the resolution function and the list of dependencies.
	// Dependencies are codes of other injectors that must be executed first.
	Resolve() (ResolveFunc, []string)

	// IsCritical indicates if an error in this injector should stop the process.
	// If false, the error is only logged and processing continues.
	IsCritical() bool

	// Timeout returns the timeout for this injector.
	// If 0, the global default timeout (30s) is used.
	Timeout() time.Duration

	// DataType returns the type of value this injector produces.
	// Used by frontend for display and validation.
	DataType() entity.ValueType

	// DefaultValue returns the default value if resolution fails.
	// Return nil for no default (error will be raised if critical).
	DefaultValue() *entity.InjectableValue

	// Formats returns the format configuration for this injector.
	// Return nil if formatting is not applicable.
	// The system will pass the selected format via InjectorContext.SelectedFormat().
	Formats() *entity.FormatConfig
}

// InitFunc is the global initialization function that runs BEFORE all injectors.
// The user defines ONE Init function that prepares shared data.
// The result (user's custom struct) will be available via InjectorContext.InitData().
type InitFunc func(ctx context.Context, injCtx *entity.InjectorContext) (any, error)

// TableSchemaProvider is an optional interface that table injectors can implement
// to expose their column structure at the API level.
// This allows the frontend to know what columns a TABLE injectable will have.
type TableSchemaProvider interface {
	// ColumnSchema returns the column definitions for this table injector.
	ColumnSchema() []entity.TableColumn
}

// ListSchemaProvider is an optional interface that list injectors can implement
// to expose their default configuration at the API level.
// This allows the frontend to know the symbol style and header labels.
type ListSchemaProvider interface {
	// ListSchema returns the list schema for this list injector.
	ListSchema() entity.ListSchema
}
