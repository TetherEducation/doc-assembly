package mappers

import (
	"context"
	"encoding/json"

	"github.com/TetherEducation/doc-assembly/core/internal/core/port"
)

// ExamplePayload is the business-specific payload.
// Define the fields you expect to receive in the request body.
type ExamplePayload struct {
	CustomerName string  `json:"customerName"`
	ProductID    string  `json:"productId"`
	Amount       float64 `json:"amount"`
	Quantity     int     `json:"quantity"`
}

// ExampleMapper is an example mapper that demonstrates how to implement
// the port.RequestMapper interface.
//
// To create a new mapper:
// 1. Define the payload struct (e.g., ExamplePayload)
// 2. Add the //docengine:mapper comment before the mapper struct
// 3. Implement the methods: Map(), ExtractInjectableValues(), Validate()
// 4. Run: make gen
//
//docengine:mapper
type ExampleMapper struct{}

// Map parses the raw body and returns the business-specific payload.
// If you need to support multiple document types, route internally here.
func (m *ExampleMapper) Map(ctx context.Context, mapCtx *port.MapperContext) (any, error) {
	var payload ExamplePayload
	if err := json.Unmarshal(mapCtx.RawBody, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}
