# Extensibility — Injectors, Mappers, Init, Resolvers

The data-shaping surface. These extensions decide *what values flow into a rendered template*. None of them touch external systems (those live in [adapters.md](adapters.md) and [signing.md](signing.md)).

## Where to register

By convention everything goes through `extensions/register.go`:

```go
package extensions

import (
    "github.com/TetherEducation/doc-assembly/core/sdk"
    "github.com/myorg/my-wrapper/extensions/injectors"
)

func Register(engine *sdk.Engine) {
    engine.SetInitFunc(myInit)
    engine.RegisterInjector(&injectors.Greeting{})
    engine.RegisterInjector(&injectors.OrderTotal{})
    engine.SetMapper(&MyMapper{})
    engine.SetTemplateResolver(&MyTemplateResolver{})
    engine.SetProcessResolver(&MyProcessResolver{})
    engine.SetWorkspaceInjectableProvider(&MyWorkspaceInjectables{})
}
```

## Injector

An injector resolves a single value at render time. Each must implement `sdk.Injector`:

```go
package injectors

import (
    "context"
    "time"

    "github.com/TetherEducation/doc-assembly/core/sdk"
)

type Greeting struct{}

func (Greeting) Code() string                       { return "greeting" }
func (Greeting) DataType() sdk.ValueType            { return sdk.ValueTypeString }
func (Greeting) DefaultValue() *sdk.InjectableValue { return nil }
func (Greeting) Formats() *sdk.FormatConfig         { return nil }
func (Greeting) IsCritical() bool                   { return false }
func (Greeting) Timeout() time.Duration             { return 0 } // 0 = global default

func (Greeting) Resolve() (sdk.ResolveFunc, []string) {
    return func(ctx context.Context, injCtx *sdk.InjectorContext) (*sdk.InjectorResult, error) {
        return &sdk.InjectorResult{Value: sdk.StringValue("Hello!")}, nil
    }, nil // no dependencies
}
```

### Method contract

| Method | Purpose | Notes |
|---|---|---|
| `Code()` | Stable, **globally unique** identifier. | Maps 1:1 with the editor token (`{{greeting}}`) and with the i18n YAML key. |
| `DataType()` | Tells the editor + validator which `sdk.ValueType` to expect. | One of `ValueTypeString`, `Number`, `Bool`, `Time`, `Table`, `Image`, `List`. |
| `DefaultValue()` | Used when `Resolve` returns an error and `IsCritical() == false`. | Return `nil` if there is no sensible fallback. |
| `Formats()` | Optional `FormatConfig` exposed in the editor (e.g., date formats). | Selected format reaches `Resolve` via `injCtx.SelectedFormat()`. |
| `IsCritical()` | If `true`, an error aborts the whole render. | Use `false` for cosmetic fields. |
| `Timeout()` | Per-injector deadline. `0` ⇒ engine default (30 s). | Keep tight; everything runs inside the request. |
| `Resolve()` | Returns `(ResolveFunc, dependencies []string)`. Dependencies are the `Code()`s that must run first. | Cycles cause boot-time validation failure. |

### Value constructors

Use `sdk.StringValue`, `NumberValue`, `BoolValue`, `TimeValue`, `ImageValue`, `NewTableValue`, `NewListValue`. See [core/sdk/types.go](../../../core/sdk/types.go).

### Optional schema interfaces (table / list)

If your injector returns a table or a list, implement the optional schema provider so the editor can render the right placeholder UI:

```go
func (t *MyTable) ColumnSchema() []sdk.TableColumn { ... } // sdk.TableSchemaProvider
func (l *MyList) ListSchema() sdk.ListSchema       { ... } // sdk.ListSchemaProvider
```

### i18n for injectors

Add an entry per injector in `settings/injectors.i18n.yaml`:

```yaml
greeting:
  label:
    en: "Greeting"
    es: "Saludo"
  description:
    en: "A greeting message"
    es: "Un mensaje de saludo"
```

The top-level key MUST match `Code()`. `en` is required; missing locales fall back to `en`.

## InitFunc — shared per-request state

Runs **once** before any injector, per render request. Whatever you return is exposed to every injector through `injCtx.InitData()` — useful for fetching a single record once and re-using it across many injectors:

```go
func myInit(ctx context.Context, injCtx *sdk.InjectorContext) (any, error) {
    return loadCustomer(ctx, injCtx.WorkspaceCode(), injCtx.ExternalID())
}

// Inside an injector's ResolveFunc:
data := injCtx.InitData().(*Customer)
return &sdk.InjectorResult{Value: sdk.StringValue(data.FullName)}, nil
```

Returning an error from `InitFunc` aborts the render.

## RequestMapper — typed payloads

Only **one** mapper is allowed. It parses the raw HTTP body into your typed payload before the rendering pipeline runs:

```go
type MyMapper struct{}

func (m *MyMapper) Map(ctx context.Context, mc *sdk.MapperContext) (any, error) {
    var p MyPayload
    if err := json.Unmarshal(mc.RawBody, &p); err != nil {
        return nil, fmt.Errorf("decoding %s: %w", mc.DocumentTypeCode, err)
    }
    return &p, nil
}
```

`MapperContext` exposes `TenantCode`, `WorkspaceCode`, `DocumentTypeCode`, `Operation`, `Environment`, `Headers`, `RawBody`, etc. Branch on `DocumentTypeCode` if you support multiple shapes — there is no separate registry of mappers.

## TemplateResolver — choose a version

Only used by the internal create flow. Use this when version selection depends on something the default resolver does not know (feature flag, signed feature toggle, A/B group):

```go
type MyTemplateResolver struct{}

func (r *MyTemplateResolver) Resolve(
    ctx context.Context,
    req *sdk.TemplateResolverRequest,
    adapter sdk.TemplateVersionSearchAdapter,
) (*string, error) {
    items, err := adapter.SearchTemplateVersions(ctx, sdk.TemplateVersionSearchParams{
        TenantCode:     req.TenantCode,
        WorkspaceCodes: []string{req.WorkspaceCode},
        DocumentType:   req.DocumentType,
        Tags:           []string{"feature-x"},
        Published:      ptr(true),
    })
    if err != nil || len(items) == 0 {
        return nil, nil // fall back to default resolver
    }
    return &items[0].VersionID, nil
}
```

Returning `(nil, nil)` is the documented escape hatch for "I do not know — let the default win".

## ProcessResolver

If your domain has a notion of *processes* (workflow phases / case types) the engine will ask for the available list and validate user input against it:

```go
type MyProcessResolver struct{}

func (r *MyProcessResolver) ListProcesses(ctx context.Context, tenantID string) ([]sdk.ProcessInfo, error) {
    return []sdk.ProcessInfo{
        {Process: "onboarding", ProcessType: "customer", Label: "Onboarding"},
        {Process: "renewal",   ProcessType: "customer", Label: "Renewal"},
    }, nil
}
func (r *MyProcessResolver) ValidateProcess(ctx context.Context, tenantID, process, processType string) error {
    // return nil to accept, error to reject
    return nil
}
```

## WorkspaceInjectableProvider

Use when you want each workspace to *publish its own* injectables that you cannot pre-compile (e.g., a tenant defines a field at runtime in your admin UI). The provider:

- Lists injectables for the editor (`GetInjectables`) — you return labels already translated for the requested locale.
- Resolves a batch of codes during render (`ResolveInjectables`) — return per-code results plus a per-code `Errors` map for non-critical failures so the render keeps going.

```go
func (p *MyProvider) GetInjectables(ctx context.Context, ic *sdk.InjectorContext) (*sdk.GetInjectablesResult, error) { ... }
func (p *MyProvider) ResolveInjectables(ctx context.Context, req *sdk.ResolveInjectablesRequest) (*sdk.ResolveInjectablesResult, error) { ... }
```

Codes returned must NOT collide with codes registered via `RegisterInjector`. A collision is treated as a configuration error at boot.

## Ordering and dependencies

- The engine builds a DAG of injectors using `Resolve()`'s dependency list and runs them in topological order.
- Within a single layer, execution is concurrent up to the engine's worker pool — do not rely on side effects between sibling injectors. Use `InitFunc` for shared state.
- Cyclic dependencies fail at boot.

## When to use what

| Need | Use |
|---|---|
| Static field with stable code, same logic per render | `Injector` |
| Field whose set varies per workspace | `WorkspaceInjectableProvider` |
| Heavy data load shared by many injectors | `InitFunc` + `injCtx.InitData()` |
| Different request shape per document type | `RequestMapper` (route inside) |
| Pick a non-default template version | `TemplateResolver` |
| Validate / list business processes | `ProcessResolver` |
