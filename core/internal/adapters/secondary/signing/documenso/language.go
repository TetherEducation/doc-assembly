package documenso

import "strings"

// documensoLanguages maps the lowercased language codes Documenso accepts in
// envelope meta (the v2 API's meta.language enum) to the form the API expects.
var documensoLanguages = map[string]string{
	"de": "de", "en": "en", "es": "es", "fr": "fr", "it": "it", "ja": "ja",
	"ko": "ko", "nl": "nl", "pl": "pl", "pt-br": "pt-BR", "zh": "zh",
}

// documensoLanguage returns the Documenso code for a document language
// ("es", "ES", "es-CL" → "es"), or "" when Documenso has no match. Documenso
// rejects an envelope whose meta.language is outside its list, so an unknown
// language is left out rather than sent.
func documensoLanguage(language string) string {
	lang := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
	if code, ok := documensoLanguages[lang]; ok {
		return code
	}
	base, _, _ := strings.Cut(lang, "-")
	return documensoLanguages[base]
}
