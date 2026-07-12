package documenso

import (
	"encoding/base64"
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

// decodeFragment reverses the encoding exactly the way Documenso's embed
// page does: JSON.parse(decodeURIComponent(atob(hash))).
// url.PathUnescape matches decodeURIComponent for %XX sequences and, unlike
// url.QueryUnescape, does not treat '+' as a space.
func decodeFragment(t *testing.T, fragment string) map[string]any {
	t.Helper()

	jsEscaped, err := base64.StdEncoding.DecodeString(fragment)
	if err != nil {
		t.Fatalf("atob failed: %v", err)
	}
	jsonStr, err := url.PathUnescape(string(jsEscaped))
	if err != nil {
		t.Fatalf("decodeURIComponent failed: %v", err)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(jsonStr), &out); err != nil {
		t.Fatalf("JSON.parse failed: %v (payload %q)", err, jsonStr)
	}
	return out
}

func TestEmbedOptionsFragmentRoundTrip(t *testing.T) {
	allowRejection := false
	opts := EmbedOptions{
		Enabled:                true,
		DarkModeDisabled:       true,
		LockName:               true,
		AllowDocumentRejection: &allowRejection,
		Language:               "es",
		CSS:                    ".embed--Root { padding: 12px; }",
		CSSVars: map[string]string{
			"primary": "#5627FF",
			"radius":  "12px",
		},
	}

	fragment, ok := opts.fragment("Adela Madrid Ávila")
	if !ok {
		t.Fatal("expected a fragment")
	}

	decoded := decodeFragment(t, fragment)

	if decoded["darkModeDisabled"] != true {
		t.Errorf("darkModeDisabled = %v, want true", decoded["darkModeDisabled"])
	}
	if decoded["name"] != "Adela Madrid Ávila" {
		t.Errorf("name = %v, want the non-ASCII name intact", decoded["name"])
	}
	if decoded["lockName"] != true {
		t.Errorf("lockName = %v, want true", decoded["lockName"])
	}
	if decoded["allowDocumentRejection"] != false {
		t.Errorf("allowDocumentRejection = %v, want false", decoded["allowDocumentRejection"])
	}
	if decoded["language"] != "es" {
		t.Errorf("language = %v, want es", decoded["language"])
	}
	if decoded["css"] != ".embed--Root { padding: 12px; }" {
		t.Errorf("css = %v", decoded["css"])
	}
	cssVars, _ := decoded["cssVars"].(map[string]any)
	if cssVars["primary"] != "#5627FF" || cssVars["radius"] != "12px" {
		t.Errorf("cssVars = %v", decoded["cssVars"])
	}
}

func TestEmbedOptionsFragmentEmpty(t *testing.T) {
	if _, ok := (EmbedOptions{}).fragment(""); ok {
		t.Error("zero options and no name must not produce a fragment")
	}

	// LockName without a name must not leak a lockName-only payload.
	if _, ok := (EmbedOptions{Enabled: true, LockName: true}).fragment("  "); ok {
		t.Error("lockName without a recipient name must not produce a fragment")
	}
}

func TestEmbedOptionsDisabledByDefault(t *testing.T) {
	// Fully configured but not enabled: URLs must stay untouched, including
	// the recipient name. Guarantees a no-op default for existing deployments.
	allowRejection := false
	opts := EmbedOptions{
		DarkModeDisabled:       true,
		LockName:               true,
		AllowDocumentRejection: &allowRejection,
		Language:               "es",
		CSS:                    "body{}",
		CSSVars:                map[string]string{"primary": "#5627FF"},
	}
	if _, ok := opts.fragment("Adela Madrid Ávila"); ok {
		t.Error("options must not produce a fragment unless Enabled is true")
	}
}

func TestEmbedOptionsFragmentNameOnly(t *testing.T) {
	fragment, ok := EmbedOptions{Enabled: true, LockName: true}.fragment("María José O'Higgins")
	if !ok {
		t.Fatal("expected a fragment for name-only options")
	}
	decoded := decodeFragment(t, fragment)
	if decoded["name"] != "María José O'Higgins" {
		t.Errorf("name = %v", decoded["name"])
	}
	if decoded["lockName"] != true {
		t.Errorf("lockName = %v, want true", decoded["lockName"])
	}
	if _, present := decoded["darkModeDisabled"]; present {
		t.Error("darkModeDisabled must be omitted when not enabled")
	}
}

func TestEmbedOptionsCanonicalizesLowercasedCSSVarKeys(t *testing.T) {
	// Viper lowercases YAML map keys; Documenso's schema is strict camelCase.
	opts := EmbedOptions{
		Enabled: true,
		CSSVars: map[string]string{
			"primary":             "#5627FF",
			"primaryforeground":   "#FFFFFF",
			"fieldcard":           "#EAEEFF",
			"fieldcardforeground": "#3E1CC5",
			"mutedforeground":     "#6E7191",
			"radius":              "12px",
			"customFutureVar":     "kept-as-is",
		},
	}
	fragment, ok := opts.fragment("")
	if !ok {
		t.Fatal("expected fragment")
	}
	decoded := decodeFragment(t, fragment)
	cssVars, _ := decoded["cssVars"].(map[string]any)
	for _, want := range []string{"primary", "primaryForeground", "fieldCard", "fieldCardForeground", "mutedForeground", "radius", "customFutureVar"} {
		if _, present := cssVars[want]; !present {
			t.Errorf("cssVars missing canonical key %q (got %v)", want, cssVars)
		}
	}
	if _, present := cssVars["primaryforeground"]; present {
		t.Error("lowercased key leaked through instead of being canonicalized")
	}
}

func TestEncodeURIComponentMatchesJavaScript(t *testing.T) {
	// Expected values produced by Node's encodeURIComponent.
	cases := map[string]string{
		`{"a":1}`:            `%7B%22a%22%3A1%7D`,
		"a b":                "a%20b",
		"a+b":                "a%2Bb",
		"ñandú":              "%C3%B1and%C3%BA",
		"-_.!~*'()":          "-_.!~*'()",
		"#5627FF; url(x=y&)": "%235627FF%3B%20url(x%3Dy%26)",
	}
	for in, want := range cases {
		if got := encodeURIComponent(in); got != want {
			t.Errorf("encodeURIComponent(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFragmentAppendedToEmbeddedURL(t *testing.T) {
	opts := EmbedOptions{Enabled: true, DarkModeDisabled: true}
	fragment, ok := opts.fragment("Adela")
	if !ok {
		t.Fatal("expected fragment")
	}
	full := "https://sign.example.com/embed/sign/tok#" + fragment
	parsed, err := url.Parse(full)
	if err != nil {
		t.Fatalf("resulting URL does not parse: %v", err)
	}
	if !strings.HasPrefix(parsed.Fragment, fragment[:8]) {
		t.Errorf("fragment lost in URL: %q", parsed.Fragment)
	}
}
