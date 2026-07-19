package documenso

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// EmbedOptions customizes Documenso's embedded signing widget. Documenso
// reads them from the URL fragment of /embed/sign/{token} as:
//
//	JSON.parse(decodeURIComponent(atob(location.hash.slice(1))))
//
// Unknown keys are stripped by Documenso's schema, so options newer than the
// deployed Documenso version degrade silently instead of breaking signing.
type EmbedOptions struct {
	// Enabled turns the options fragment on. When false (the default) the
	// embedded URL stays byte-identical to previous releases, including
	// leaving the signer name untouched.
	Enabled bool

	// DarkModeDisabled pins the widget to light mode regardless of the
	// signer's OS preference.
	DarkModeDisabled bool

	// LockName pre-fills and locks the signer name field when the recipient
	// name is known, instead of asking the signer to type it.
	LockName bool

	// AllowDocumentRejection toggles the reject affordance. Nil leaves the
	// provider default untouched.
	AllowDocumentRejection *bool

	// Language selects the widget UI language (e.g. "es").
	Language string

	// CSS is raw CSS injected into the widget for whitelabel adjustments.
	CSS string

	// CSSVars maps Documenso theme variables (primary, radius, fieldCard…)
	// to brand values.
	CSSVars map[string]string
}

// fragment renders the options (plus the recipient name, when known) as the
// encoded URL fragment Documenso expects. The second return is false when
// there is nothing to append, keeping bare URLs byte-identical to the
// pre-options behavior.
func (o EmbedOptions) fragment(recipientName string) (string, bool) {
	if !o.Enabled {
		return "", false
	}

	data := map[string]any{}

	if o.DarkModeDisabled {
		data["darkModeDisabled"] = true
	}
	if name := strings.TrimSpace(recipientName); name != "" {
		data["name"] = name
		if o.LockName {
			data["lockName"] = true
		}
	}
	if o.AllowDocumentRejection != nil {
		data["allowDocumentRejection"] = *o.AllowDocumentRejection
	}
	if lang := strings.TrimSpace(o.Language); lang != "" {
		data["language"] = lang
	}
	if css := strings.TrimSpace(o.CSS); css != "" {
		data["css"] = css
	}
	if vars := canonicalCSSVars(o.CSSVars); len(vars) > 0 {
		data["cssVars"] = vars
	}

	if len(data) == 0 {
		return "", false
	}

	raw, err := json.Marshal(data)
	if err != nil {
		// Options are cosmetic: never fail URL generation over them.
		return "", false
	}

	return base64.StdEncoding.EncodeToString([]byte(encodeURIComponent(string(raw)))), true
}

// documensoCSSVarKeys maps lowercase forms to Documenso's canonical camelCase
// cssVars keys. Config loaders (Viper/mapstructure) lowercase YAML map keys,
// and Documenso's schema silently strips unknown casings — without this,
// vars like primaryForeground or fieldCard would never reach the widget.
var documensoCSSVarKeys = map[string]string{
	"background":               "background",
	"foreground":               "foreground",
	"muted":                    "muted",
	"mutedforeground":          "mutedForeground",
	"popover":                  "popover",
	"popoverforeground":        "popoverForeground",
	"card":                     "card",
	"cardborder":               "cardBorder",
	"cardbordertint":           "cardBorderTint",
	"cardforeground":           "cardForeground",
	"fieldcard":                "fieldCard",
	"fieldcardborder":          "fieldCardBorder",
	"fieldcardforeground":      "fieldCardForeground",
	"widget":                   "widget",
	"widgetforeground":         "widgetForeground",
	"border":                   "border",
	"input":                    "input",
	"primary":                  "primary",
	"primaryforeground":        "primaryForeground",
	"secondary":                "secondary",
	"secondaryforeground":      "secondaryForeground",
	"accent":                   "accent",
	"accentforeground":         "accentForeground",
	"destructive":              "destructive",
	"destructiveforeground":    "destructiveForeground",
	"ring":                     "ring",
	"radius":                   "radius",
	"warning":                  "warning",
	"envelopeeditorbackground": "envelopeEditorBackground",
}

// canonicalCSSVars restores Documenso's canonical key casing. Keys that are
// not known Documenso vars pass through unchanged (future-proof for values
// supplied with exact casing, e.g. via the JSON env override).
func canonicalCSSVars(vars map[string]string) map[string]string {
	if len(vars) == 0 {
		return nil
	}
	out := make(map[string]string, len(vars))
	for key, value := range vars {
		if canonical, ok := documensoCSSVarKeys[strings.ToLower(key)]; ok {
			out[canonical] = value
		} else {
			out[key] = value
		}
	}
	return out
}

// encodeURIComponent mirrors JavaScript's encodeURIComponent byte-for-byte
// (unreserved set: A-Z a-z 0-9 - _ . ! ~ * ' ( )) so the payload survives
// Documenso's decodeURIComponent exactly. url.QueryEscape is not equivalent:
// it encodes spaces as '+', which decodeURIComponent does not fold back.
func encodeURIComponent(s string) string {
	const unreserved = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.!~*'()"

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if strings.IndexByte(unreserved, c) >= 0 {
			b.WriteByte(c)
		} else {
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}
