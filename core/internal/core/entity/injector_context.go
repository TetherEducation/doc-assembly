package entity

import (
	"strings"
	"sync"
	"time"
)

// ValueType indicates the type of an injectable value.
type ValueType int

const (
	// ValueTypeString represents a string value.
	ValueTypeString ValueType = iota
	// ValueTypeNumber represents a numeric value (float64).
	ValueTypeNumber
	// ValueTypeBool represents a boolean value.
	ValueTypeBool
	// ValueTypeTime represents a time.Time value.
	ValueTypeTime
	// ValueTypeTable represents a table value with columns and rows.
	ValueTypeTable
	// ValueTypeImage represents an image URL value.
	ValueTypeImage
	// ValueTypeList represents a list value with items and styling.
	ValueTypeList
)

// InjectableValue is the typed value returned by an injector.
// Only allows: string, number (float64), bool, time.Time, TableValue, ListValue.
type InjectableValue struct {
	typ      ValueType
	strVal   string
	numVal   float64
	boolVal  bool
	timeVal  time.Time
	tableVal *TableValue
	listVal  *ListValue
}

// StringValue creates an InjectableValue of type string.
func StringValue(s string) InjectableValue {
	return InjectableValue{typ: ValueTypeString, strVal: s}
}

// NumberValue creates an InjectableValue of type number.
func NumberValue(n float64) InjectableValue {
	return InjectableValue{typ: ValueTypeNumber, numVal: n}
}

// BoolValue creates an InjectableValue of type bool.
func BoolValue(b bool) InjectableValue {
	return InjectableValue{typ: ValueTypeBool, boolVal: b}
}

// TimeValue creates an InjectableValue of type time.
func TimeValue(t time.Time) InjectableValue {
	return InjectableValue{typ: ValueTypeTime, timeVal: t}
}

// TableValueData creates an InjectableValue of type table.
func TableValueData(t *TableValue) InjectableValue {
	return InjectableValue{typ: ValueTypeTable, tableVal: t}
}

// ImageValue creates an InjectableValue of type image (URL string).
func ImageValue(url string) InjectableValue {
	return InjectableValue{typ: ValueTypeImage, strVal: url}
}

// ListValueData creates an InjectableValue of type list.
func ListValueData(l *ListValue) InjectableValue {
	return InjectableValue{typ: ValueTypeList, listVal: l}
}

// Type returns the type of the value.
func (v InjectableValue) Type() ValueType {
	return v.typ
}

// String returns the value as string. ok=false if not a string.
func (v InjectableValue) String() (string, bool) {
	if v.typ != ValueTypeString {
		return "", false
	}
	return v.strVal, true
}

// Number returns the value as float64. ok=false if not a number.
func (v InjectableValue) Number() (float64, bool) {
	if v.typ != ValueTypeNumber {
		return 0, false
	}
	return v.numVal, true
}

// Bool returns the value as bool. ok=false if not a bool.
func (v InjectableValue) Bool() (bool, bool) {
	if v.typ != ValueTypeBool {
		return false, false
	}
	return v.boolVal, true
}

// Time returns the value as time.Time. ok=false if not a time.
func (v InjectableValue) Time() (time.Time, bool) {
	if v.typ != ValueTypeTime {
		return time.Time{}, false
	}
	return v.timeVal, true
}

// Table returns the value as *TableValue. ok=false if not a table.
func (v InjectableValue) Table() (*TableValue, bool) {
	if v.typ != ValueTypeTable {
		return nil, false
	}
	return v.tableVal, true
}

// List returns the value as *ListValue. ok=false if not a list.
func (v InjectableValue) List() (*ListValue, bool) {
	if v.typ != ValueTypeList {
		return nil, false
	}
	return v.listVal, true
}

// AsAny returns the value as any (for rendering).
func (v InjectableValue) AsAny() any {
	switch v.typ {
	case ValueTypeString:
		return v.strVal
	case ValueTypeNumber:
		return v.numVal
	case ValueTypeBool:
		return v.boolVal
	case ValueTypeTime:
		return v.timeVal
	case ValueTypeTable:
		return v.tableVal
	case ValueTypeImage:
		return v.strVal
	case ValueTypeList:
		return v.listVal
	default:
		return nil
	}
}

// InjectorResult is the result of resolving an injector.
type InjectorResult struct {
	Value    InjectableValue
	Metadata map[string]any // optional, for logging/debug
}

// InjectorContext encapsulates context data with thread-safe access.
type InjectorContext struct {
	mu              sync.RWMutex
	externalID      string
	templateID      string
	transactionalID string
	operation       string
	tenantCode      string
	workspaceCode   string
	documentType    string
	environment     Environment
	headers         map[string]string
	resolvedValues  map[string]any
	requestPayload  any
	initData        any
	selectedFormats map[string]string // injector code -> selected format
}

// normalizeHeaders converts all header keys to lowercase for case-insensitive lookup.
func normalizeHeaders(headers map[string]string) map[string]string {
	if headers == nil {
		return nil
	}
	normalized := make(map[string]string, len(headers))
	for k, v := range headers {
		normalized[strings.ToLower(k)] = v
	}
	return normalized
}

// NewInjectorContext creates a new InjectorContext instance.
func NewInjectorContext(
	externalID, templateID, transactionalID string,
	op string,
	environment Environment,
	headers map[string]string,
	payload any,
) *InjectorContext {
	return &InjectorContext{
		externalID:      externalID,
		templateID:      templateID,
		transactionalID: transactionalID,
		operation:       op,
		environment:     environment,
		headers:         normalizeHeaders(headers),
		resolvedValues:  make(map[string]any),
		requestPayload:  payload,
	}
}

// NewInjectorContextWithCodes creates a new InjectorContext with tenant/workspace codes.
func NewInjectorContextWithCodes(
	externalID, templateID, transactionalID string,
	op string,
	tenantCode, workspaceCode string,
	environment Environment,
	headers map[string]string,
	payload any,
) *InjectorContext {
	return &InjectorContext{
		externalID:      externalID,
		templateID:      templateID,
		transactionalID: transactionalID,
		operation:       op,
		tenantCode:      tenantCode,
		workspaceCode:   workspaceCode,
		environment:     environment,
		headers:         normalizeHeaders(headers),
		resolvedValues:  make(map[string]any),
		requestPayload:  payload,
	}
}

// ExternalID returns the external ID of the request.
func (c *InjectorContext) ExternalID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.externalID
}

// TemplateID returns the template ID.
func (c *InjectorContext) TemplateID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.templateID
}

// TransactionalID returns the transactional ID.
func (c *InjectorContext) TransactionalID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.transactionalID
}

// Operation returns the operation type.
func (c *InjectorContext) Operation() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.operation
}

// TenantCode returns the tenant code.
func (c *InjectorContext) TenantCode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tenantCode
}

// WorkspaceCode returns the workspace code.
func (c *InjectorContext) WorkspaceCode() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.workspaceCode
}

// DocumentType returns the document type code.
func (c *InjectorContext) DocumentType() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.documentType
}

// SetTenantCode sets the tenant code (internal use).
func (c *InjectorContext) SetTenantCode(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tenantCode = code
}

// SetWorkspaceCode sets the workspace code (internal use).
func (c *InjectorContext) SetWorkspaceCode(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.workspaceCode = code
}

// SetDocumentType sets the document type code (internal use).
func (c *InjectorContext) SetDocumentType(code string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.documentType = code
}

// Environment returns the environment (dev/prod).
func (c *InjectorContext) Environment() Environment {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.environment
}

// SetEnvironment sets the environment (internal use).
func (c *InjectorContext) SetEnvironment(env Environment) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.environment = env
}

// Header returns the value of a specific header (case-insensitive).
func (c *InjectorContext) Header(key string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.headers == nil {
		return ""
	}
	return c.headers[strings.ToLower(key)]
}

// GetHeaders returns a copy of all headers.
func (c *InjectorContext) GetHeaders() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.headers == nil {
		return nil
	}
	result := make(map[string]string, len(c.headers))
	for k, v := range c.headers {
		result[k] = v
	}
	return result
}

// GetResolved returns the resolved value of another injector.
func (c *InjectorContext) GetResolved(code string) (any, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	val, ok := c.resolvedValues[code]
	return val, ok
}

// RequestPayload returns the request payload.
func (c *InjectorContext) RequestPayload() any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.requestPayload
}

// InitData returns the initialization data.
func (c *InjectorContext) InitData() any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.initData
}

// SetResolved stores the resolved value of an injector (internal use by resolver).
func (c *InjectorContext) SetResolved(code string, value any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resolvedValues[code] = value
}

// SetInitData stores the initialization data (internal use by resolver).
func (c *InjectorContext) SetInitData(data any) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.initData = data
}

// SelectedFormat returns the format selected by user for a specific injector.
// Returns empty string if no format is selected.
func (c *InjectorContext) SelectedFormat(code string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.selectedFormats == nil {
		return ""
	}
	return c.selectedFormats[code]
}

// SetSelectedFormats stores the selected formats (internal use by resolver).
func (c *InjectorContext) SetSelectedFormats(formats map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.selectedFormats = formats
}

// GetSelectedFormats returns a copy of all selected formats.
func (c *InjectorContext) GetSelectedFormats() map[string]string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.selectedFormats == nil {
		return nil
	}
	result := make(map[string]string, len(c.selectedFormats))
	for k, v := range c.selectedFormats {
		result[k] = v
	}
	return result
}
