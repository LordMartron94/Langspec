package spec

import (
	"langspec"
)

/*
BuildLexerSpec creates the LexerSpec and LexingRuleset from the language spec.
Iterates spec.Tokens and adds a rule for each token with Pattern != nil.
*/
func BuildLexerSpec(spec LanguageSpec) (
	*langspec.LexerSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState],
	*langspec.LexerRuleset[LangSpecLexerTokenType, LangSpecLexerTokenRole],
) {
	lexerSpec := langspec.LexerSpecCreate[
		rune,
		LangSpecLexerTokenType,
		LangSpecLexerTokenRole](
		TokEOF,
		LangSpecLexerStateInitial,
		func(t LangSpecLexerTokenType) string { return t.String() },
		true, // The parser is not mutating lexer spec.
	)

	rs := langspec.LexerRulesetCreate[
		LangSpecLexerTokenType,
		LangSpecLexerTokenRole,
	]()

	for _, tok := range spec.Tokens {
		if tok.Pattern != nil {
			rs.WithRulePriority(*tok.Pattern, tok.Type, tok.Role, tok.Priority)
			if tok.Open != nil && tok.Close != nil {
				rs.WithDelimitedRule(*tok.Open, *tok.Close, tok.Type)
			}
		}
	}

	lexerSpec.WithRuleset(LangSpecLexerStateInitial, *rs)
	return lexerSpec, rs
}

func buildLangSpecDSLLexerSpec(spec LanguageSpec) (
	*langspec.LexerSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState],
	*langspec.LexerRuleset[LangSpecLexerTokenType, LangSpecLexerTokenRole],
) {
	return BuildLexerSpec(spec)
}
