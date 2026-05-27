package index

import (
	"regexp"
	"strings"
)

var tokenSplitRe = regexp.MustCompile(`[^a-z0-9]+`)

// stopWords contains common English function words that carry no search value
// in a technical notes corpus. Tech keywords (go, get, new, time, use, may, etc.)
// are intentionally excluded — they are meaningful in this domain.
var stopWords = map[string]struct{}{
	"the": {}, "and": {}, "for": {}, "are": {}, "but": {}, "not": {},
	"you": {}, "all": {}, "can": {}, "had": {}, "her": {}, "was": {},
	"one": {}, "our": {}, "day": {}, "has": {},
	"him": {}, "his": {}, "its": {}, "now": {},
	"old": {}, "two": {}, "way": {}, "who": {},
	"did": {}, "she": {}, "each": {}, "from": {}, "have": {},
	"that": {}, "this": {}, "they": {}, "will": {}, "with": {}, "been": {},
	"into": {}, "more": {}, "also": {}, "than": {}, "then": {}, "when": {},
	"what": {}, "some": {}, "your": {}, "would": {}, "there": {},
	"their": {}, "which": {}, "about": {}, "could": {}, "other": {}, "these": {},
	"were": {}, "said": {}, "does": {}, "just": {}, "know": {},
	"take": {}, "only": {}, "come": {},
	"very": {}, "after": {}, "those": {}, "where": {}, "being": {}, "while": {},
	"should": {}, "before": {}, "through": {}, "because": {},
	"to": {}, "of": {}, "in": {}, "is": {}, "it": {}, "be": {},
	"as": {}, "at": {}, "by": {}, "we": {}, "an": {}, "or": {}, "if": {},
	"do": {}, "so": {}, "up": {}, "no": {}, "my": {}, "me": {},
	"on": {}, "he": {}, "us": {}, "am": {},
}

// Tokenize splits text into lowercase tokens of ≥2 chars,
// splitting on any non-alphanumeric character, excluding stop words.
func Tokenize(text string) []string {
	text = strings.ToLower(text)
	tokens := tokenSplitRe.Split(text, -1)
	var result []string
	for _, token := range tokens {
		if len(token) < 2 {
			continue
		}
		if _, ok := stopWords[token]; ok {
			continue
		}
		result = append(result, token)
	}
	return result
}
