package sdk

import "github.com/TetherEducation/doc-assembly/core/cmd/api/bootstrap"

// Engine is the main entry point for doc-assembly.
// Create with New(), register extensions, then call Run().
type Engine = bootstrap.Engine

// New creates a new Engine with default configuration.
var New = bootstrap.New

// NewWithConfig creates a new Engine that loads config from the given file path.
var NewWithConfig = bootstrap.NewWithConfig
