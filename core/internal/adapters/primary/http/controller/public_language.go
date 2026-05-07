package controller

import "strings"

func explicitPublicLanguage(raw string) string {
	language := strings.ToLower(strings.TrimSpace(raw))
	if language == "" {
		return ""
	}
	if language == "en" || language == "es" {
		return language
	}
	return "en"
}

func appendPublicLanguage(rawURL, language string) string {
	language = explicitPublicLanguage(language)
	if language == "" {
		return rawURL
	}
	separator := "?"
	if strings.Contains(rawURL, "?") {
		separator = "&"
	}
	return rawURL + separator + "language=" + language
}
