// Package common provides shared utilities for PostgreSQL repositories.
package common

import (
	"time"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
)

// SafeString returns the string value or empty string if nil.
func SafeString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// SafeTime returns the time value or zero time if nil.
func SafeTime(t *time.Time) time.Time {
	if t == nil {
		return time.Time{}
	}
	return *t
}

// SafeDataType returns the data type value or TEXT if nil.
func SafeDataType(dt *entity.InjectableDataType) entity.InjectableDataType {
	if dt == nil {
		return entity.InjectableDataTypeText
	}
	return *dt
}
