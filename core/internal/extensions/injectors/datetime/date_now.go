package datetime

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/formatter"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// DateNowInjector returns the current date.
//
//docengine:injector
type DateNowInjector struct{}

func (i *DateNowInjector) Code() string { return "date_now" }

func (i *DateNowInjector) DataType() entity.ValueType { return entity.ValueTypeTime }

func (i *DateNowInjector) DefaultValue() *entity.InjectableValue { return nil }

func (i *DateNowInjector) Formats() *entity.FormatConfig {
	return &entity.FormatConfig{
		Default: "DD/MM/YYYY",
		Options: []string{"DD/MM/YYYY", "MM/DD/YYYY", "YYYY-MM-DD", "long"},
	}
}

func (i *DateNowInjector) Resolve() (port.ResolveFunc, []string) {
	return func(ctx context.Context, injCtx *entity.InjectorContext) (*entity.InjectorResult, error) {
		now := time.Now()
		format := injCtx.SelectedFormat("date_now")
		if format == "" {
			format = "DD/MM/YYYY"
		}

		var formatted string
		if format == "long" {
			formatted = now.Format("2 January 2006")
		} else {
			formatted = formatter.FormatTime(now, format)
		}

		return &entity.InjectorResult{Value: entity.StringValue(formatted)}, nil
	}, nil
}

func (i *DateNowInjector) IsCritical() bool       { return false }
func (i *DateNowInjector) Timeout() time.Duration { return 0 }
