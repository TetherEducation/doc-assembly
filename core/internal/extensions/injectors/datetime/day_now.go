package datetime

import (
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// DayNowInjector returns the current day of the month.
//
//docengine:injector
type DayNowInjector struct {
	nowNumberInjectorBase
}

func (i *DayNowInjector) Code() string { return "day_now" }

func (i *DayNowInjector) Resolve() (port.ResolveFunc, []string) {
	return resolveNowNumber(extractDayNow)
}

func extractDayNow(now time.Time) float64 {
	return float64(now.Day())
}
