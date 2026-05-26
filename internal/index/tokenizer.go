package index

import (
	"regexp"
	"strings"
)

var tokenSplitRe = regexp.MustCompile(`[^a-z0-9]+`)

// Tokenize splits text into lowercase tokens of ≥2 chars,
// splitting on any non-alphanumeric character.
func Tokenize(text string) []string {
	// Lowercase the text
	text = strings.ToLower(text)

	// Split on non-alphanumeric characters
	tokens := tokenSplitRe.Split(text, -1)

	// Filter tokens with len < 2
	var result []string
	for _, token := range tokens {
		if len(token) >= 2 {
			result = append(result, token)
		}
	}

	return result
}
