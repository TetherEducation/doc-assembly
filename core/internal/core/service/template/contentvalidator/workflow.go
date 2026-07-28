package contentvalidator

import (
	"fmt"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
)

// validateWorkflow validates the signing workflow configuration.
func (s *Service) validateWorkflow(vctx *validationContext) {
	workflow := vctx.doc.SigningWorkflow

	// Workflow is optional
	if workflow == nil {
		return
	}

	// Validate order mode
	if !portabledoc.ValidOrderModes.Contains(workflow.OrderMode) {
		vctx.addErrorf(ErrCodeInvalidOrderMode, "signingWorkflow.orderMode",
			"Invalid order mode: %s. Must be 'parallel' or 'sequential'", workflow.OrderMode)
	}

	// Validate notifications
	validateNotificationConfig(vctx, &workflow.Notifications, workflow.OrderMode)
}

// validateNotificationConfig validates notification configuration.
func validateNotificationConfig(
	vctx *validationContext,
	config *portabledoc.NotificationConfig,
	orderMode string,
) {
	path := "signingWorkflow.notifications"

	// Validate scope
	if !portabledoc.ValidNotifyScopes.Contains(config.Scope) {
		vctx.addErrorf(ErrCodeInvalidNotifyScope, path+".scope",
			"Invalid notification scope: %s. Must be 'global' or 'individual'", config.Scope)
	}

	// Validate global triggers if scope is global
	if config.Scope == portabledoc.NotifyScopeGlobal {
		validateTriggerMap(vctx, config.GlobalTriggers, path+".globalTriggers", orderMode)
	}

	// Validate role configs if scope is individual
	if config.Scope == portabledoc.NotifyScopeIndividual {
		for i, roleConfig := range config.RoleConfigs {
			rolePath := fmt.Sprintf("%s.roleConfigs[%d]", path, i)
			validateRoleNotifyConfig(vctx, roleConfig, rolePath, orderMode)
		}
	}
}

// validateTriggerMap validates a map of notification triggers.
func validateTriggerMap(
	vctx *validationContext,
	triggers portabledoc.TriggerMap,
	path string,
	orderMode string,
) {
	for triggerType, settings := range triggers {
		triggerPath := fmt.Sprintf("%s.%s", path, triggerType)

		if settings == nil {
			continue
		}

		// Check if trigger is valid for the order mode
		if orderMode == portabledoc.OrderModeParallel {
			if portabledoc.SequentialOnlyTriggers.Contains(triggerType) {
				vctx.addErrorf(ErrCodeSequentialTriggerError, triggerPath,
					"Trigger '%s' is only valid for sequential order mode", triggerType)
			}
		}

		// Validate previousRolesConfig if present
		if settings.PreviousRolesConfig != nil {
			validatePreviousRolesConfig(vctx, settings.PreviousRolesConfig, triggerPath+".previousRolesConfig")
		}
	}
}

// validateRoleNotifyConfig validates notification config for a specific role.
func validateRoleNotifyConfig(
	vctx *validationContext,
	roleConfig portabledoc.RoleNotifyConfig,
	path string,
	orderMode string,
) {
	// Validate roleId exists
	if roleConfig.RoleID == "" {
		vctx.addError(ErrCodeInvalidRoleConfigRef, path+".roleId",
			"Role notification config must have a roleId")
		return
	}

	if !vctx.roleIDSet.Contains(roleConfig.RoleID) {
		vctx.addErrorf(ErrCodeInvalidRoleConfigRef, path+".roleId",
			"Role notification config references unknown role: %s", roleConfig.RoleID)
	}

	// Validate triggers
	validateTriggerMap(vctx, roleConfig.Triggers, path+".triggers", orderMode)
}

// validatePreviousRolesConfig validates previous roles configuration.
func validatePreviousRolesConfig(
	vctx *validationContext,
	config *portabledoc.PreviousRolesConf,
	path string,
) {
	// Validate mode
	if !portabledoc.ValidPreviousRolesModes.Contains(config.Mode) {
		vctx.addErrorf(ErrCodeInvalidPreviousRoleMode, path+".mode",
			"Invalid previous roles mode: %s. Must be 'auto' or 'custom'", config.Mode)
	}

	// If mode is custom, validate selected role IDs
	if config.Mode == portabledoc.PreviousRolesModeCustom {
		for i, roleID := range config.SelectedRoleIDs {
			if !vctx.roleIDSet.Contains(roleID) {
				vctx.addErrorf(ErrCodeInvalidPreviousRoleRef,
					fmt.Sprintf("%s.selectedRoleIds[%d]", path, i),
					"Previous roles config references unknown role: %s", roleID)
			}
		}
	}
}
