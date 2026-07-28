package datetime

import (
	"context"
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

type nowNumberInjectorBase struct{}

func (nowNumberInjectorBase) DataType() entity.ValueType { return entity.ValueTypeNumber }

func (nowNumberInjectorBase) DefaultValue() *entity.InjectableValue { return nil }

func (nowNumberInjectorBase) Formats() *entity.FormatConfig { return nil }

func (nowNumberInjectorBase) IsCritical() bool { return false }

func (nowNumberInjectorBase) Timeout() time.Duration { return 0 }

func resolveNowNumber(extractor func(time.Time) float64) (port.ResolveFunc, []string) {
	return func(_ context.Context, _ *entity.InjectorContext) (*entity.InjectorResult, error) {
		return &entity.InjectorResult{
			Value: entity.NumberValue(extractor(time.Now())),
		}, nil
	}, nil
}
