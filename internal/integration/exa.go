package integration

import (
	"strconv"
	"strings"
)

// BuildExaRequestBody transforms flat string params into the typed, nested JSON
// structure that the Exa API expects. This handles:
//   - Integer coercion for numResults
//   - Boolean coercion for moderation
//   - Comma-separated strings → arrays for includeDomains, excludeDomains,
//     includeText, excludeText, and ids
//   - Default contents.highlights injection for search/find_similar actions
func BuildExaRequestBody(action string, params map[string]string) map[string]interface{} {
	body := make(map[string]interface{})

	// Integer params
	intParams := map[string]bool{"numResults": true}
	// Array params (comma-separated → []string)
	arrayParams := map[string]bool{
		"includeDomains": true,
		"excludeDomains": true,
		"includeText":    true,
		"excludeText":    true,
		"ids":            true,
	}
	// Boolean params
	boolParams := map[string]bool{"moderation": true}

	for k, v := range params {
		switch {
		case intParams[k]:
			if n, err := strconv.Atoi(v); err == nil {
				body[k] = n
			} else {
				body[k] = v
			}
		case arrayParams[k]:
			body[k] = splitCSV(v)
		case boolParams[k]:
			body[k] = v == "true"
		default:
			body[k] = v
		}
	}

	// Inject default contents.highlights for search and find_similar
	// unless the caller already set contents explicitly.
	if action == "search" || action == "find_similar" {
		if _, hasContents := body["contents"]; !hasContents {
			body["contents"] = map[string]interface{}{
				"highlights": true,
			}
		}
	}

	// Inject default contents.text for get_contents
	if action == "get_contents" {
		if _, hasContents := body["contents"]; !hasContents {
			body["contents"] = map[string]interface{}{
				"text": true,
			}
		}
	}

	return body
}

// splitCSV splits a comma-separated string into trimmed, non-empty parts.
func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
