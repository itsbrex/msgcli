package text

import "strings"

// StripHTML performs basic HTML tag removal for display.
func StripHTML(s string) string {
	// This is a very basic implementation
	// For production, consider a proper HTML-to-text library
	result := s

	// Remove common block elements with newlines
	for _, tag := range []string{"</p>", "</div>", "</br>", "<br>", "<br/>", "<br />"} {
		result = strings.ReplaceAll(result, tag, "\n")
	}

	// Remove all remaining tags
	inTag := false
	var out strings.Builder
	for _, r := range result {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			out.WriteRune(r)
		}
	}

	// Decode common HTML entities
	result = out.String()
	result = strings.ReplaceAll(result, "&nbsp;", " ")
	result = strings.ReplaceAll(result, "&amp;", "&")
	result = strings.ReplaceAll(result, "&lt;", "<")
	result = strings.ReplaceAll(result, "&gt;", ">")
	result = strings.ReplaceAll(result, "&quot;", "\"")
	result = strings.ReplaceAll(result, "&#39;", "'")

	// Collapse multiple newlines
	for strings.Contains(result, "\n\n\n") {
		result = strings.ReplaceAll(result, "\n\n\n", "\n\n")
	}

	return strings.TrimSpace(result)
}
