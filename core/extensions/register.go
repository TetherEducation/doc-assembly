package extensions

import (
	"github.com/TetherEducation/doc-assembly/core/cmd/api/bootstrap"
	internalext "github.com/TetherEducation/doc-assembly/core/internal/extensions"
	"github.com/TetherEducation/doc-assembly/core/internal/extensions/injectors"
	"github.com/TetherEducation/doc-assembly/core/internal/extensions/mappers"
)

// Register configures all user-defined extensions on the engine.
// Edit this function to add custom injectors, mappers, and providers.
func Register(engine *bootstrap.Engine) {
	// Init function (runs before all injectors on each render request)
	engine.SetInitFunc(internalext.GlobalInit(&internalext.InitDeps{}))

	// Example injectors (replace with your own)
	engine.RegisterInjector(&injectors.ExampleInjector{})
	engine.RegisterInjector(&injectors.ExampleTableInjector{})
	engine.RegisterInjector(&injectors.ExampleImageInjector{})

	// Example mapper (replace with your own)
	engine.SetMapper(&mappers.ExampleMapper{})
}
