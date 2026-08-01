// Package plaintext converts rich text content into readable terminal text.
package plaintext

import (
	"html"
	"regexp"
	"strings"
)

var (
	htmlScriptStyleRE = regexp.MustCompile(`(?is)<(script|style)[^>]*>.*?</(script|style)>`)
	htmlBreakRE       = regexp.MustCompile(`(?i)<(br|/p|/div|/tr|/li|/h[1-6])[^>]*>`)
	htmlTagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	htmlBlankRE       = regexp.MustCompile(`\n{3,}`)
)

// HTMLToText turns HTML into readable plain text.
func HTMLToText(s string) string {
	s = htmlScriptStyleRE.ReplaceAllString(s, "")
	s = htmlBreakRE.ReplaceAllString(s, "\n")
	s = htmlTagRE.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = strings.ReplaceAll(s, "\r", "")
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(line, " \t")
	}
	s = strings.Join(lines, "\n")
	s = htmlBlankRE.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}
