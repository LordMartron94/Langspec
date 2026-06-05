package sublime

import (
	"testing"

	"autarch/pattern"
)

/*
TestLiteralPrefixOverlapAlphabet verifies that a literal which is a strict prefix of a sibling
literal is forbidden from being immediately followed by that sibling's differentiating character.
This is what lets Sublime's first-match engine reproduce the DFA's maximal munch for overlapping
quote literals (`"` / `""` / `"""`), instead of the shorter prefix shadowing the longer one.
*/
func TestLiteralPrefixOverlapAlphabet(t *testing.T) {
	quote, doubleQuote, tripleQuote := `"`, `""`, `"""`
	siblings := []string{quote, doubleQuote, tripleQuote}

	cases := []struct {
		name      string
		literal   string
		wantGuard bool
		wantRegex string
	}{
		{
			name:      "single quote is extended by longer quote literals",
			literal:   quote,
			wantGuard: true,
			wantRegex: `(?![\"])`,
		},
		{
			name:      "empty string is extended by triple quote",
			literal:   doubleQuote,
			wantGuard: true,
			wantRegex: `(?![\"])`,
		},
		{
			name:      "longest literal needs no guard",
			literal:   tripleQuote,
			wantGuard: false,
		},
		{
			name:      "no overlapping siblings yields no guard",
			literal:   "target",
			wantGuard: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			alpha, ok := literalPrefixOverlapAlphabet(tc.literal, siblings)
			if ok != tc.wantGuard {
				t.Fatalf("literalPrefixOverlapAlphabet(%q) guard presence = %v, want %v", tc.literal, ok, tc.wantGuard)
			}
			if !tc.wantGuard {
				return
			}
			guard, err := pattern.PatternAlphabetToNegativeLookahead(alpha)
			if err != nil {
				t.Fatalf("PatternAlphabetToNegativeLookahead: %v", err)
			}
			if guard != tc.wantRegex {
				t.Fatalf("guard for %q = %q, want %q", tc.literal, guard, tc.wantRegex)
			}
		})
	}
}
