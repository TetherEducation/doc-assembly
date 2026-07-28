package contentvalidator

import (
	"fmt"

	"github.com/expr-lang/expr"

	"github.com/TetherEducation/doc-assembly/core/internal/core/entity/portabledoc"
)

// validateConditionals validates all conditional blocks in the document.
func (s *Service) validateConditionals(vctx *validationContext) {
	doc := vctx.doc

	// Collect and validate all conditional nodes
	for i, node := range doc.NodesOfType(portabledoc.NodeTypeConditional) {
		path := fmt.Sprintf("content.conditional[%d]", i)
		validateConditionalNode(vctx, node, path, s.maxNestingDepth)
	}
}

// validateConditionalNode validates a single conditional block node.
func validateConditionalNode(vctx *validationContext, node portabledoc.Node, path string, maxDepth int) {
	attrs, err := portabledoc.ParseConditionalAttrs(node.Attrs)
	if err != nil {
		vctx.addErrorf(ErrCodeInvalidConditionAttrs, path+".attrs",
			"Invalid conditional attributes: %s", err.Error())
		return
	}

	// Validate the logic group recursively
	validateLogicGroup(vctx, &attrs.Conditions, path+".conditions", 0, maxDepth)

	// Validate expression syntax with expr-lang
	if attrs.Expression != "" {
		validateExpressionSyntax(vctx, attrs.Expression, path+".expression")
	}
}

// validateLogicGroup validates a logic group recursively.
func validateLogicGroup(
	vctx *validationContext,
	group *portabledoc.LogicGroup,
	path string,
	depth int,
	maxDepth int,
) {
	// Check nesting depth
	if depth > maxDepth {
		vctx.addWarningf(ErrCodeMaxNestingExceeded, path,
			"Condition nesting exceeds maximum depth of %d", maxDepth)
		return
	}

	// Validate group type
	if group.Type != portabledoc.LogicTypeGroup {
		vctx.addErrorf(ErrCodeInvalidLogicOperator, path+".type",
			"Expected type 'group', got: %s", group.Type)
	}

	// Validate logic operator
	if !portabledoc.ValidLogicOperators.Contains(group.Logic) {
		vctx.addErrorf(ErrCodeInvalidLogicOperator, path+".logic",
			"Logic must be AND or OR, got: %s", group.Logic)
	}

	// Validate group has at least one child
	if len(group.Children) == 0 {
		vctx.addError(ErrCodeEmptyConditionGroup, path+".children",
			"Condition group must have at least one rule or nested group")
		return
	}

	// Validate children
	for i, child := range group.Children {
		childPath := fmt.Sprintf("%s.children[%d]", path, i)
		validateLogicChild(vctx, child, childPath, depth+1, maxDepth)
	}
}

// validateLogicChild validates a child of a logic group (rule or nested group).
func validateLogicChild(
	vctx *validationContext,
	child any,
	path string,
	depth int,
	maxDepth int,
) {
	// Try to parse as rule first
	rule, err := portabledoc.ParseLogicRule(child)
	if err == nil && rule.Type == portabledoc.LogicTypeRule {
		validateLogicRule(vctx, rule, path)
		return
	}

	// Try as nested group
	group, err := portabledoc.ParseLogicGroup(child)
	if err == nil && group.Type == portabledoc.LogicTypeGroup {
		validateLogicGroup(vctx, group, path, depth, maxDepth)
		return
	}

	// Invalid child type
	vctx.addError(ErrCodeInvalidConditionAttrs, path,
		"Child must be a 'rule' or 'group'")
}

// validateLogicRule validates a single logic rule.
func validateLogicRule(vctx *validationContext, rule *portabledoc.LogicRule, path string) {
	// Validate type
	if rule.Type != portabledoc.LogicTypeRule {
		vctx.addErrorf(ErrCodeInvalidConditionAttrs, path+".type",
			"Expected type 'rule', got: %s", rule.Type)
	}

	// Validate variableId exists
	if rule.VariableID == "" {
		vctx.addError(ErrCodeInvalidConditionVar, path+".variableId",
			"Rule variableId is required")
	} else if !vctx.variableSet.Contains(rule.VariableID) {
		vctx.addErrorf(ErrCodeInvalidConditionVar, path+".variableId",
			"Variable '%s' not found in document variables or role variables", rule.VariableID)
	}

	// Validate operator
	if rule.Operator == "" {
		vctx.addError(ErrCodeInvalidOperator, path+".operator",
			"Operator is required")
	} else if !portabledoc.ValidOperators.Contains(rule.Operator) {
		vctx.addErrorf(ErrCodeInvalidOperator, path+".operator",
			"Invalid operator: %s", rule.Operator)
	}

	// Validate value mode
	if !portabledoc.ValidRuleModes.Contains(rule.Value.Mode) {
		vctx.addErrorf(ErrCodeInvalidRuleValueMode, path+".value.mode",
			"Invalid value mode: %s. Must be 'text' or 'variable'", rule.Value.Mode)
	}

	// If comparing to another variable, validate it exists
	if rule.Value.Mode == portabledoc.RuleModeVariable && rule.Value.Value != "" {
		if !vctx.variableSet.Contains(rule.Value.Value) {
			vctx.addErrorf(ErrCodeInvalidConditionVar, path+".value.value",
				"Comparison variable '%s' not found", rule.Value.Value)
		}
	}

	// Operators that don't require a value shouldn't have one
	if portabledoc.NoValueOperators.Contains(rule.Operator) && rule.Value.Value != "" {
		vctx.addWarningf(WarnCodeExpressionWarning, path+".value.value",
			"Operator '%s' doesn't require a value, but one was provided", rule.Operator)
	}

	// Operators that require a value must have one
	if rule.Operator != "" && !portabledoc.NoValueOperators.Contains(rule.Operator) && rule.Value.Value == "" {
		vctx.addErrorf(ErrCodeMissingConditionValue, path+".value.value",
			"Operator '%s' requires a comparison value", rule.Operator)
	}
}

// validateExpressionSyntax validates that the expression can be compiled by expr-lang.
func validateExpressionSyntax(vctx *validationContext, expression string, path string) {
	// Build a minimal environment for compilation check
	// We just need to verify syntax, not actual evaluation
	env := buildExprEnvironment(vctx.variableSet)

	// Attempt to compile the expression
	_, err := expr.Compile(expression, expr.Env(env), expr.AsBool())
	if err != nil {
		// This is a warning because the expression format from frontend uses symbols
		// that may not directly translate to expr-lang syntax
		vctx.addWarningf(WarnCodeExpressionWarning, path,
			"Expression may have syntax issues: %s", err.Error())
	}
}

// buildExprEnvironment builds an environment map for expr compilation.
func buildExprEnvironment(variableSet portabledoc.Set[string]) map[string]any {
	env := make(map[string]any, variableSet.Len())
	for varID := range variableSet {
		// Provide placeholder values for type inference
		env[varID] = ""
	}
	return env
}
