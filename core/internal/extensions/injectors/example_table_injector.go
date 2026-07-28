package injectors

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// ExampleTableInjector demonstrates how to create a table injector.
// This is for DEMONSTRATION PURPOSES ONLY - do not use in production.
//
// To create a real table injector:
//  1. Copy this file as a starting point
//  2. Modify Code() to return your unique identifier
//  3. Implement Resolve() to build your table from InitData or RequestPayload
//  4. Add translations in settings/injectors.i18n.yaml
//  5. Run: make gen
//
//docengine:injector
type ExampleTableInjector struct{}

const exampleTableCode = "example_table"

// Code returns the unique identifier of the injector.
func (i *ExampleTableInjector) Code() string {
	return exampleTableCode
}

// DataType returns the type of value this injector produces.
func (i *ExampleTableInjector) DataType() entity.ValueType {
	return entity.ValueTypeTable
}

// Resolve returns the resolution function and the list of dependencies.
func (i *ExampleTableInjector) Resolve() (port.ResolveFunc, []string) {
	return func(ctx context.Context, injCtx *entity.InjectorContext) (*entity.InjectorResult, error) {
		// EXAMPLE: Static demo data
		// In production, use injCtx.InitData() or injCtx.RequestPayload()
		_ = injCtx.InitData() // available for real implementations

		table := entity.NewTableValue().
			AddColumn("item", map[string]string{
				"es": "Item",
				"en": "Item",
			}, entity.ValueTypeString).
			AddColumn("description", map[string]string{
				"es": "Descripción",
				"en": "Description",
			}, entity.ValueTypeString).
			AddColumn("value", map[string]string{
				"es": "Valor",
				"en": "Value",
			}, entity.ValueTypeNumber).
			// Demo rows
			AddRow(
				entity.Cell(entity.StringValue("A")),
				entity.Cell(entity.StringValue("First example item")),
				entity.Cell(entity.NumberValue(100.00)),
			).
			AddRow(
				entity.Cell(entity.StringValue("B")),
				entity.Cell(entity.StringValue("Second example item")),
				entity.Cell(entity.NumberValue(200.00)),
			).
			AddRow(
				entity.Cell(entity.StringValue("C")),
				entity.Cell(entity.StringValue("Third example item")),
				entity.Cell(entity.NumberValue(300.00)),
			).
			WithHeaderStyles(entity.TableStyles{
				Background: entity.StringPtr("#f0f0f0"),
				FontWeight: entity.StringPtr("bold"),
				TextAlign:  entity.StringPtr("center"),
			})

		return &entity.InjectorResult{
			Value: entity.TableValueData(table),
		}, nil
	}, nil // no dependencies on other injectors
}

// IsCritical indicates if an error in this injector should stop the process.
func (i *ExampleTableInjector) IsCritical() bool {
	return false
}

// Timeout returns the timeout for this injector.
func (i *ExampleTableInjector) Timeout() time.Duration {
	return 0
}

// DefaultValue returns the default value if resolution fails.
func (i *ExampleTableInjector) DefaultValue() *entity.InjectableValue {
	return nil
}

// Formats returns the format configuration for this injector.
func (i *ExampleTableInjector) Formats() *entity.FormatConfig {
	return nil
}

// ColumnSchema returns the column definitions for this table injector.
// Implements port.TableSchemaProvider.
func (i *ExampleTableInjector) ColumnSchema() []entity.TableColumn {
	return []entity.TableColumn{
		{
			Key:      "item",
			Labels:   map[string]string{"es": "Item", "en": "Item"},
			DataType: entity.ValueTypeString,
		},
		{
			Key:      "description",
			Labels:   map[string]string{"es": "Descripción", "en": "Description"},
			DataType: entity.ValueTypeString,
		},
		{
			Key:      "value",
			Labels:   map[string]string{"es": "Valor", "en": "Value"},
			DataType: entity.ValueTypeNumber,
		},
	}
}
