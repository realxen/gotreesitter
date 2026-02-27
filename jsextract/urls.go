package jsextract

import "strings"

// MaybeURL returns true if s looks like a URL or API path worth extracting.
func MaybeURL(s string) bool {
	if len(s) < 5 {
		return false
	}

	// Absolute URLs
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return true
	}

	// Protocol-relative URLs
	if strings.HasPrefix(s, "//") && len(s) > 2 && s[2] != '/' {
		return true
	}

	// Absolute paths (must contain at least one more char after /)
	if s[0] == '/' && len(s) > 1 && s[1] != '/' && s[1] != '*' {
		return true
	}

	// Reject common false positives
	if isFalsePositive(s) {
		return false
	}

	// Relative paths with path separator (e.g., "api/v1/users")
	if strings.Contains(s, "/") && !strings.HasPrefix(s, "/*") {
		// Must have word-like segments
		parts := strings.SplitN(s, "/", 3)
		if len(parts) >= 2 && len(parts[0]) > 0 && len(parts[1]) > 0 {
			return true
		}
	}

	return false
}

func isFalsePositive(s string) bool {
	// CSS selectors
	if s[0] == '.' || s[0] == '#' || s[0] == ':' {
		return true
	}
	// Template literals / expressions
	if strings.Contains(s, "${") {
		return true
	}
	// MIME types
	if strings.HasPrefix(s, "text/") || strings.HasPrefix(s, "application/") ||
		strings.HasPrefix(s, "image/") || strings.HasPrefix(s, "multipart/") {
		return true
	}
	// Common non-URL patterns
	if strings.HasPrefix(s, "data:") || strings.HasPrefix(s, "javascript:") ||
		strings.HasPrefix(s, "mailto:") {
		return true
	}
	// Date patterns like "2024/01/01"
	if len(s) >= 4 && isDigit(s[0]) && isDigit(s[1]) && isDigit(s[2]) && isDigit(s[3]) {
		return true
	}
	return false
}

func isDigit(c byte) bool {
	return c >= '0' && c <= '9'
}

// cleanURL trims whitespace and removes surrounding quotes from a URL string.
func cleanURL(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			s = s[1 : len(s)-1]
		}
	}
	return s
}
