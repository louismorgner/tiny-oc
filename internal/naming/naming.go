package naming

import (
	"strings"
	"unicode"
)

// FromPrompt generates a short human-readable name from a task prompt.
// Returns empty string if no prompt is provided.
func FromPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return ""
	}

	// Take first line only
	if idx := strings.IndexByte(prompt, '\n'); idx != -1 {
		prompt = prompt[:idx]
	}

	// Lowercase and split into words
	prompt = strings.ToLower(prompt)
	words := strings.Fields(prompt)
	if len(words) == 0 {
		return ""
	}

	// Cap at 6 words
	if len(words) > 6 {
		words = words[:6]
	}

	name := strings.Join(words, " ")
	name = cleanName(name)

	// Truncate to 50 chars max without cutting mid-word
	if len(name) > 50 {
		name = name[:50]
		if idx := strings.LastIndexByte(name, ' '); idx > 0 {
			name = name[:idx]
		}
	}

	return name
}

func cleanName(name string) string {
	// Remove surrounding quotes
	name = strings.Trim(name, `"'`)
	// Remove trailing punctuation (keep hyphens and slashes)
	name = strings.TrimRightFunc(name, func(r rune) bool {
		return unicode.IsPunct(r) && r != '-' && r != '/'
	})
	return strings.TrimSpace(name)
}
