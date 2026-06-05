package sublime

import (
	"strings"
	"unicode"

	"autarch/pattern"
	"cmp"
	"foundation/domain"
	"langspec/editor"
)

var boundaryRegexFactory = pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())

func collectDynamicPatternASTs[TObservation cmp.Ordered, TContext any](
	transitions []editor.EditorTransition[TObservation, TContext],
) []pattern.RegulaAST[rune] {
	if !observationIsRune[TObservation]() {
		return nil
	}
	var out []pattern.RegulaAST[rune]
	for _, tr := range transitions {
		if transitionExcludedFromBoundaryInference(tr) {
			continue
		}
		if ast, ok := transitionDynamicPatternRune(tr); ok {
			out = append(out, ast)
		}
	}
	return out
}

/*
synthesizeTransitionLiteralGuard returns a single (?![...]) negative lookahead that makes a literal
transition safe under Sublime Text's first-match (leftmost-PEG) regex engine, which — unlike the
runtime DFA lexer — does not perform maximal munch.

[Context]
Two independent disambiguation hazards exist when a context emits a pure-literal match:
 1. Word boundary: a keyword literal (e.g. `in`) must not fire inside a longer identifier matched
    by a dynamic sibling pattern (e.g. `inputs`). Resolved via DFA prefix continuation against the
    context's dynamic patterns (pattern.PatternLiteralBoundaryChars).
 2. Prefix overlap: a literal that is a strict prefix of a sibling literal (e.g. `""` vs `"""`, or
    `"` vs `""`) must not fire when the longer literal applies. Sublime tries rules top-to-bottom
    and takes the first match, so the shorter prefix would otherwise win and shadow the longer one.
    Resolved by forbidding the differentiating next characters of every longer sibling literal.

Both hazards reduce to "the literal must not be immediately followed by character X", so their
forbidden-character alphabets are unioned into one lookahead. Word boundary inference applies only to
[A-Za-z0-9_] literals; prefix-overlap inference applies to every pure literal, punctuation included.

[Side Effects]
Pure function. Does not mutate inputs.
*/
func synthesizeTransitionLiteralGuard[TObservation cmp.Ordered, TContext any](
	tr editor.EditorTransition[TObservation, TContext],
	dynamicASTs []pattern.RegulaAST[rune],
	contextLiterals []string,
) string {
	if transitionExcludedFromBoundaryInference(tr) {
		return ""
	}
	literal, ok := transitionPureLiteralString(tr)
	if !ok {
		return ""
	}

	var alphabets []pattern.PatternAlphabet[rune]
	if isWordLiteralText(literal) {
		if alpha, ok := literalBoundaryAlphabetForWord(literal, dynamicASTs); ok {
			alphabets = append(alphabets, alpha)
		}
	}
	if alpha, ok := literalPrefixOverlapAlphabet(literal, contextLiterals); ok {
		alphabets = append(alphabets, alpha)
	}
	if len(alphabets) == 0 {
		return ""
	}

	guard, err := pattern.PatternAlphabetToNegativeLookahead(pattern.PatternAlphabetUnion(alphabets...))
	if err != nil {
		return ""
	}
	return guard
}

/*
literalBoundaryAlphabetForWord returns the union of characters that any dynamic sibling pattern can
use to extend word w beyond |w|, i.e. the characters w must not be followed by to remain a standalone
keyword. Returns false when no dynamic pattern can extend w.

[Algorithmic Approach]
For each dynamic regex R, FirstChars(w,R) is computed via DFA prefix continuation
(pattern.PatternLiteralBoundaryChars). This is a sound one-character over-approximation: coaccessible
continuation symbols are exact for a single negative-lookahead step, though pathological regexes may
over-approximate.

[Side Effects]
Pure function. Does not mutate inputs.
*/
func literalBoundaryAlphabetForWord(
	word string,
	dynamicASTs []pattern.RegulaAST[rune],
) (pattern.PatternAlphabet[rune], bool) {
	if word == "" || len(dynamicASTs) == 0 {
		return pattern.PatternAlphabet[rune]{}, false
	}

	var parts []pattern.PatternAlphabet[rune]
	for _, ast := range dynamicASTs {
		alpha, ok := pattern.PatternLiteralBoundaryChars(word, ast)
		if !ok {
			continue
		}
		parts = append(parts, alpha)
	}
	if len(parts) == 0 {
		return pattern.PatternAlphabet[rune]{}, false
	}
	return pattern.PatternAlphabetUnion(parts...), true
}

/*
literalPrefixOverlapAlphabet returns the set of characters that immediately follow literal in any
sibling literal that has literal as a strict prefix. Forbidding these characters via a negative
lookahead prevents the shorter literal from shadowing a longer sibling under Sublime's first-match
engine (e.g. `""` must not match the opening of `"""`). Returns false when literal is not a strict
prefix of any sibling.

[Side Effects]
Pure function. Does not mutate inputs.
*/
func literalPrefixOverlapAlphabet(
	literal string,
	siblingLiterals []string,
) (pattern.PatternAlphabet[rune], bool) {
	if literal == "" {
		return pattern.PatternAlphabet[rune]{}, false
	}

	seen := map[rune]bool{}
	var ranges []pattern.CharRange[rune]
	for _, sibling := range siblingLiterals {
		if len(sibling) <= len(literal) || !strings.HasPrefix(sibling, literal) {
			continue
		}
		next := []rune(sibling[len(literal):])[0]
		if seen[next] {
			continue
		}
		seen[next] = true
		ranges = append(ranges, pattern.CharRange[rune]{Lo: next, Hi: next})
	}
	if len(ranges) == 0 {
		return pattern.PatternAlphabet[rune]{}, false
	}
	return pattern.PatternAlphabetCollect(boundaryRegexFactory.Class(ranges...))
}

/*
collectPureLiteralStrings returns the raw literal text of every pure-literal transition eligible for
boundary inference, used as the sibling set for prefix-overlap disambiguation. Lookahead transitions
and transitions already carrying a negative lookahead are excluded, mirroring
transitionExcludedFromBoundaryInference.

[Side Effects]
Pure function. Does not mutate inputs.
*/
func collectPureLiteralStrings[TObservation cmp.Ordered, TContext any](
	transitions []editor.EditorTransition[TObservation, TContext],
) []string {
	var out []string
	for _, tr := range transitions {
		if transitionExcludedFromBoundaryInference(tr) {
			continue
		}
		if literal, ok := transitionPureLiteralString(tr); ok {
			out = append(out, literal)
		}
	}
	return out
}

/*
transitionPureLiteralString extracts the raw literal text of a pure-literal transition, decoded from
the source runes rather than the regex form so that prefix comparison and next-character extraction
are not corrupted by regex escaping. Returns false for non-literal, non-rune, or empty transitions.

[Side Effects]
Pure function. Does not mutate inputs.
*/
func transitionPureLiteralString[TObservation cmp.Ordered, TContext any](
	tr editor.EditorTransition[TObservation, TContext],
) (string, bool) {
	if !observationIsRune[TObservation]() {
		return "", false
	}
	if tr.RegexPattern != nil {
		if !regexPatternIsPlainLiteral(*tr.RegexPattern) {
			return "", false
		}
		return *tr.RegexPattern, true
	}
	if !pattern.PatternIsPureLiteral(tr.OnPattern) {
		return "", false
	}
	ast, ok := any(tr.OnPattern).(pattern.RegulaAST[rune])
	if !ok {
		return "", false
	}

	var runes []rune
	ast.Accept(pattern.RegulaVisitor[rune]{
		VisitLiteral: func(values []rune) { runes = append(runes, values...) },
	})
	if len(runes) == 0 {
		return "", false
	}
	return string(runes), true
}

func isWordLiteralText(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			continue
		}
		if r >= 'a' && r <= 'z' {
			continue
		}
		if r >= '0' && r <= '9' || r == '_' {
			continue
		}
		return false
	}
	return true
}

func transitionExcludedFromBoundaryInference[TObservation cmp.Ordered, TContext any](
	tr editor.EditorTransition[TObservation, TContext],
) bool {
	if tr.IsLookahead {
		return true
	}
	if tr.RegexPattern != nil && strings.Contains(*tr.RegexPattern, "(?!") {
		return true
	}
	if tr.RegexPattern == nil {
		re, err := tr.OnPattern.ToRegEx()
		if err == nil && strings.Contains(re, "(?!") {
			return true
		}
	}
	return false
}

func transitionDynamicPatternRune[TObservation cmp.Ordered, TContext any](
	tr editor.EditorTransition[TObservation, TContext],
) (pattern.RegulaAST[rune], bool) {
	if tr.RegexPattern != nil {
		if regexPatternIsPlainLiteral(*tr.RegexPattern) {
			return pattern.RegulaAST[rune]{}, false
		}
		ast, err := pattern.RegexToRegula(*tr.RegexPattern, boundaryRegexFactory)
		if err != nil {
			return pattern.RegulaAST[rune]{}, false
		}
		return ast, true
	}
	if pattern.PatternIsPureLiteral(tr.OnPattern) {
		return pattern.RegulaAST[rune]{}, false
	}
	ast, ok := any(tr.OnPattern).(pattern.RegulaAST[rune])
	return ast, ok
}

func regexPatternIsPlainLiteral(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch r {
		case '[', '(', ')', '*', '+', '?', '{', '}', '|', '\\', '.', '^', '$':
			return false
		default:
			if unicode.IsControl(r) {
				return false
			}
		}
	}
	return true
}

func applyLiteralBoundaryGuard(baseRegex, guard string) string {
	if guard == "" || baseRegex == "" {
		return baseRegex
	}
	if strings.Contains(baseRegex, "(?!") {
		return baseRegex
	}
	return baseRegex + guard
}

func observationIsRune[TObservation cmp.Ordered]() bool {
	var zero TObservation
	_, ok := any(zero).(rune)
	return ok
}
