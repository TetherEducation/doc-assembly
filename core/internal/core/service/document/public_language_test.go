package document

import "testing"

func TestAppendSigningURLLanguage(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		language string
		want     string
	}{
		{name: "missing language", path: "/public/sign/token", want: "/public/sign/token"},
		{name: "english", path: "/public/sign/token", language: "en", want: "/public/sign/token?language=en"},
		{name: "spanish with existing query", path: "/public/sign/token?x=1", language: "es", want: "/public/sign/token?x=1&language=es"},
		{name: "unsupported explicit fallback", path: "/public/sign/token", language: "pt", want: "/public/sign/token?language=en"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := appendSigningURLLanguage(tt.path, tt.language); got != tt.want {
				t.Fatalf("appendSigningURLLanguage() = %q, want %q", got, tt.want)
			}
		})
	}
}
