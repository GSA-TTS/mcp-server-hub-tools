package main

import "testing"

func TestNormalizeMultilineSecret(t *testing.T) {
	tests := map[string]struct {
		input string
		want  string
	}{
		"escaped newlines": {
			input: `-----BEGIN PRIVATE KEY-----\nkey material\n-----END PRIVATE KEY-----\n`,
			want:  "-----BEGIN PRIVATE KEY-----\nkey material\n-----END PRIVATE KEY-----\n",
		},
		"real newlines": {
			input: "line one\nline two\n",
			want:  "line one\nline two\n",
		},
	}

	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			if got := normalizeMultilineSecret(tt.input); got != tt.want {
				t.Fatalf("normalizeMultilineSecret() = %q, want %q", got, tt.want)
			}
		})
	}
}
