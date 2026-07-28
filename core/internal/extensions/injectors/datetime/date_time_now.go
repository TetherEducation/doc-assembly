package datetime

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/formatter"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// DateTimeNowInjector returns the current date and time.
//
//docengine:injector
type DateTimeNowInjector struct{}

func (i *DateTimeNowInjector) Code() string { return "date_time_now" }

func (i *DateTimeNowInjector) DataType() entity.ValueType { return entity.ValueTypeTime }

func (i *DateTimeNowInjector) DefaultValue() *entity.InjectableValue { return nil }

func (i *DateTimeNowInjector) Formats() *entity.FormatConfig {
	return &entity.FormatConfig{
		Default: "DD/MM/YYYY HH:mm",
		Options: []string{"DD/MM/YYYY HH:mm", "YYYY-MM-DD HH:mm:ss", "long"},
	}
}

func (i *DateTimeNowInjector) Resolve() (port.ResolveFunc, []string) {
	return func(ctx context.Context, injCtx *entity.InjectorContext) (*entity.InjectorResult, error) {
		now := time.Now()
		format := injCtx.SelectedFormat("date_time_now")
		if format == "" {
			format = "DD/MM/YYYY HH:mm"
		}

		var formatted string
		if format == "long" {
			formatted = now.Format("2 January 2006, 15:04")
		} else {
			formatted = formatter.FormatTime(now, format)
		}

		return &entity.InjectorResult{Value: entity.StringValue(formatted)}, nil
	}, nil
}

func (i *DateTimeNowInjector) IsCritical() bool       { return false }
func (i *DateTimeNowInjector) Timeout() time.Duration { return 0 }
