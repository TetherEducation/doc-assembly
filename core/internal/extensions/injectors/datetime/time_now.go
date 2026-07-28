package datetime

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/formatter"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// TimeNowInjector returns the current time.
//
//docengine:injector
type TimeNowInjector struct{}

func (i *TimeNowInjector) Code() string { return "time_now" }

func (i *TimeNowInjector) DataType() entity.ValueType { return entity.ValueTypeTime }

func (i *TimeNowInjector) DefaultValue() *entity.InjectableValue { return nil }

func (i *TimeNowInjector) Formats() *entity.FormatConfig {
	return &entity.FormatConfig{
		Default: "HH:mm",
		Options: []string{"HH:mm", "HH:mm:ss", "hh:mm a"},
	}
}

func (i *TimeNowInjector) Resolve() (port.ResolveFunc, []string) {
	return func(ctx context.Context, injCtx *entity.InjectorContext) (*entity.InjectorResult, error) {
		now := time.Now()
		format := injCtx.SelectedFormat("time_now")
		if format == "" {
			format = "HH:mm"
		}

		formatted := formatter.FormatTime(now, format)
		return &entity.InjectorResult{Value: entity.StringValue(formatted)}, nil
	}, nil
}

func (i *TimeNowInjector) IsCritical() bool       { return false }
func (i *TimeNowInjector) Timeout() time.Duration { return 0 }
