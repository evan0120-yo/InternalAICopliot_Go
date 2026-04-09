package promptguard

import (
	"regexp"
	"strings"

	"golang.org/x/text/width"
)

var whitespacePattern = regexp.MustCompile(`\s+`)

func normalizeText(raw string) string {
	normalized := strings.Map(removeZeroWidthRune, raw)
	normalized = width.Narrow.String(normalized)
	normalized = strings.ReplaceAll(normalized, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ToLower(normalized)
	normalized = whitespacePattern.ReplaceAllString(normalized, " ")
	return strings.TrimSpace(normalized)
}

func removeZeroWidthRune(r rune) rune {
	switch r {
	case '\u200B', '\u200C', '\u200D', '\u2060', '\uFEFF':
		return -1
	default:
		return r
	}
}
