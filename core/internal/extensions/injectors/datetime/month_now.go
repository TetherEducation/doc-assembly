package datetime

import (
	"context"
	"strconv"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// MonthNowInjector returns the current month.
//
//docengine:injector
type MonthNowInjector struct{}

func (i *MonthNowInjector) Code() string { return "month_now" }

func (i *MonthNowInjector) DataType() entity.ValueType { return entity.ValueTypeNumber }

func (i *MonthNowInjector) DefaultValue() *entity.InjectableValue { return nil }

func (i *MonthNowInjector) Formats() *entity.FormatConfig {
	return &entity.FormatConfig{
		Default: "number",
		Options: []string{"number", "name", "short_name"},
	}
}

func (i *MonthNowInjector) Resolve() (port.ResolveFunc, []string) {
	return func(ctx context.Context, injCtx *entity.InjectorContext) (*entity.InjectorResult, error) {
		now := time.Now()
		format := injCtx.SelectedFormat("month_now")
		if format == "" {
			format = "number"
		}

		var result entity.InjectableValue
		switch format {
		case "name":
			result = entity.StringValue(now.Format("January"))
		case "short_name":
			result = entity.StringValue(now.Format("Jan"))
		default: // "number"
			result = entity.StringValue(strconv.Itoa(int(now.Month())))
		}

		return &entity.InjectorResult{Value: result}, nil
	}, nil
}

func (i *MonthNowInjector) IsCritical() bool       { return false }
func (i *MonthNowInjector) Timeout() time.Duration { return 0 }
