package extensions

import (
	"context"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// InitDeps contains the dependencies that the user needs for Init.
// The user must define the fields according to their needs.
type InitDeps struct {
	// Add required services here, for example:
	// CRMService    CRMService
	// ConfigService ConfigService
}

// InitializedData is the custom struct that contains all the data
// loaded in Init() and available to all injectors.
// The user defines the structure according to their needs.
type InitializedData struct {
	// Add fields as needed, for example:
	// ClientInfo  *ClientInfo
	// ProductInfo *ProductInfo
	// Config      *WorkspaceConfig
}

// GlobalInit is the initialization function that runs BEFORE all injectors.
// It loads shared data that will be available to all injectors via InitData().
//
//docengine:init
func GlobalInit(deps *InitDeps) port.InitFunc {
	return func(ctx context.Context, injCtx *entity.InjectorContext) (any, error) {
		// TODO: Implement the initialization logic.
		// Example:
		// 1. Load client data from CRM using injCtx.ExternalID()
		// 2. Load workspace configuration using injCtx.WorkspaceID()
		// 3. Return a struct with all loaded data

		return &InitializedData{}, nil
	}
}
