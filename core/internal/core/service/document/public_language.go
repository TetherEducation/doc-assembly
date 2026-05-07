package document

import "strings"

func appendSigningURLLanguage(path, language string) string {
	language = strings.ToLower(strings.TrimSpace(language))
	if language == "" {
		return path
	}
	if language != "en" && language != "es" {
		language = "en"
	}
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	return path + separator + "language=" + language
}
