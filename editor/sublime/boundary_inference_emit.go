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
synthesizeLiteralBoundaryGuardForWord returns (?![FirstChars(w,R1|R2|...)]) for one word literal w.

For each dynamic regex R in the context, FirstChars(w,R) is computed via DFA prefix continuation
(pattern.PatternLiteralBoundaryChars). Only R that can extend w beyond |w| contribute; unrelated
rules yield empty and are skipped. This is a sound one-character over-approximation: coaccessible
continuation symbols are exact for a single negative-lookahead step, but pathological regexes may
still over-approximate. Guards apply only to [A-Za-z0-9_] word literals, not punctuation or strings.
*/
func synthesizeLiteralBoundaryGuardForWord(
	word string,
	dynamicASTs []pattern.RegulaAST[rune],
) string {
	if word == "" || len(dynamicASTs) == 0 {
		return ""
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
		return ""
	}
	combined := parts[0]
	for i := 1; i < len(parts); i++ {
		combined = pattern.PatternAlphabetUnion(combined, parts[i])
	}
	guard, err := pattern.PatternAlphabetToNegativeLookahead(combined)
	if err != nil {
		return ""
	}
	return guard
}

func transitionWordLiteral[TObservation cmp.Ordered, TContext any](
	tr editor.EditorTransition[TObservation, TContext],
) (string, bool) {
	if !observationIsRune[TObservation]() {
		return "", false
	}
	if tr.RegexPattern != nil {
		if !regexPatternIsPlainLiteral(*tr.RegexPattern) {
			return "", false
		}
		if !isWordLiteralText(*tr.RegexPattern) {
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
	word := patternLiteralString(ast)
	if word == "" || !isWordLiteralText(word) {
		return "", false
	}
	return word, true
}

func patternLiteralString(ast pattern.RegulaAST[rune]) string {
	// PatternIsPureLiteral implies literal runes only; read via ToRegEx is wrong.
	// Walk not exported — use Regex from literals by compiling small helper in pattern package.
	re, err := ast.ToRegEx()
	if err != nil {
		return ""
	}
	// ToRegEx on pure literal may escape; for ASCII keywords re equals text.
	return re
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
