package datetime

import (
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// YearNowInjector returns the current year.
//
//docengine:injector
type YearNowInjector struct {
	nowNumberInjectorBase
}

func (i *YearNowInjector) Code() string { return "year_now" }

func (i *YearNowInjector) Resolve() (port.ResolveFunc, []string) {
	return resolveNowNumber(extractYearNow)
}

func extractYearNow(now time.Time) float64 {
	return float64(now.Year())
}
