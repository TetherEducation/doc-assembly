package documenso

import "testing"

func TestProviderFieldType(t *testing.T) {
	if providerFieldType("text") != "TEXT" || providerFieldType("") != "SIGNATURE" || providerFieldType("nope") != "SIGNATURE" {
		t.Fatalf("text=%s empty=%s nope=%s", providerFieldType("text"), providerFieldType(""), providerFieldType("nope"))
	}
}
