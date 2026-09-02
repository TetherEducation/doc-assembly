package sdk

import (
	"github.com/TetherEducation/doc-assembly/core/internal/core/entity"
	documentuc "github.com/TetherEducation/doc-assembly/core/internal/core/usecase/document"
)

// --- Core Context & Results ---

// InjectorContext provides context for injector execution (workspace, operation, etc.).
type InjectorContext = entity.InjectorContext

// InjectorResult holds the resolved value from an injector.
type InjectorResult = entity.InjectorResult

// InjectableValue wraps a typed value (string, number, table, etc.).
type InjectableValue = entity.InjectableValue

// CreateReadOnlyViewLinkResult is the expiring public Read-Only View Link
// returned by Engine.CreateReadOnlyViewLinkByWorkspaceCode. URL is the view
// page; byte fetch is GET {URL}/pdf (token only, no Authorization).
type CreateReadOnlyViewLinkResult = documentuc.CreateReadOnlyViewLinkResult

// --- Value Type Enum ---

// ValueType identifies the type of an InjectableValue.
type ValueType = entity.ValueType

// Value type constants.
const (
	ValueTypeString = entity.ValueTypeString
	ValueTypeNumber = entity.ValueTypeNumber
	ValueTypeBool   = entity.ValueTypeBool
	ValueTypeTime   = entity.ValueTypeTime
	ValueTypeTable  = entity.ValueTypeTable
	ValueTypeImage  = entity.ValueTypeImage
	ValueTypeList   = entity.ValueTypeList
)

// --- Value Constructors ---

// StringValue creates a string InjectableValue.
var StringValue = entity.StringValue

// NumberValue creates a numeric InjectableValue.
var NumberValue = entity.NumberValue

// BoolValue creates a boolean InjectableValue.
var BoolValue = entity.BoolValue

// TimeValue creates a time InjectableValue.
var TimeValue = entity.TimeValue

// TableValueData creates a table InjectableValue.
var TableValueData = entity.TableValueData

// ImageValue creates an image InjectableValue.
var ImageValue = entity.ImageValue

// ListValueData creates a list InjectableValue.
var ListValueData = entity.ListValueData

// --- Table Types ---

// TableValue represents tabular data with columns and rows.
type TableValue = entity.TableValue

// TableColumn defines a column in a table.
type TableColumn = entity.TableColumn

// TableCell represents a single cell in a table.
type TableCell = entity.TableCell

// TableRow is a row of cells.
type TableRow = entity.TableRow

// TableStyles controls table rendering appearance.
type TableStyles = entity.TableStyles

// Table constructors.
var (
	NewTableValue = entity.NewTableValue
	Cell          = entity.Cell
	CellWithSpan  = entity.CellWithSpan
	EmptyCell     = entity.EmptyCell
)

// --- List Types ---

// ListValue represents a list with items.
type ListValue = entity.ListValue

// ListItem represents an item in a list (can be nested).
type ListItem = entity.ListItem

// ListStyles controls list rendering appearance.
type ListStyles = entity.ListStyles

// ListSchema defines the schema for a list injectable.
type ListSchema = entity.ListSchema

// ListSymbol identifies the bullet/number style.
type ListSymbol = entity.ListSymbol

// List constructors.
var (
	NewListValue   = entity.NewListValue
	ListItemValue  = entity.ListItemValue
	ListItemNested = entity.ListItemNested
)

// --- Formatting ---

// FormatConfig controls how injectable values are formatted.
type FormatConfig = entity.FormatConfig

// --- Operation Type Enum ---

// OperationType identifies the document operation (create, renew, amend, etc.).
type OperationType = entity.OperationType

// Operation type constants.
const (
	OperationCreate  = entity.OperationCreate
	OperationRenew   = entity.OperationRenew
	OperationAmend   = entity.OperationAmend
	OperationCancel  = entity.OperationCancel
	OperationPreview = entity.OperationPreview
)

// --- Injectable Data Type Enum ---

// InjectableDataType identifies the data type of an injectable definition.
type InjectableDataType = entity.InjectableDataType

// --- Document Status Enum ---

// DocumentStatus represents the lifecycle state of a document.
type DocumentStatus = entity.DocumentStatus

// Document status constants.
const (
	DocumentStatusDraft              = entity.DocumentStatusDraft
	DocumentStatusAwaitingInput      = entity.DocumentStatusAwaitingInput
	DocumentStatusPreparingSignature = entity.DocumentStatusPreparingSignature
	DocumentStatusReadyToSign        = entity.DocumentStatusReadyToSign
	DocumentStatusSigning            = entity.DocumentStatusSigning
	DocumentStatusCompleted          = entity.DocumentStatusCompleted
	DocumentStatusDeclined           = entity.DocumentStatusDeclined
	DocumentStatusCancelled          = entity.DocumentStatusCancelled
	DocumentStatusInvalidated        = entity.DocumentStatusInvalidated
	DocumentStatusError              = entity.DocumentStatusError
)

// --- Recipient Status Enum ---

// RecipientStatus represents the signing state of a recipient.
type RecipientStatus = entity.RecipientStatus

// Recipient status constants.
const (
	RecipientStatusPending   = entity.RecipientStatusPending
	RecipientStatusSent      = entity.RecipientStatusSent
	RecipientStatusDelivered = entity.RecipientStatusDelivered
	RecipientStatusSigned    = entity.RecipientStatusSigned
	RecipientStatusDeclined  = entity.RecipientStatusDeclined
)
