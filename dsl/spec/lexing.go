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
		LANG_SPEC_LEXER_STATE_DEFAULT,
		func(r rune) string { return string(r) },
		nil,
		func(t LangSpecLexerTokenType) string { return t.String() },
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

	lexerSpec.WithRuleset(LANG_SPEC_LEXER_STATE_DEFAULT, *rs)
	return lexerSpec, rs
}

func buildLangSpecDSLLexerSpec(spec LanguageSpec) (
	*langspec.LexerSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState],
	*langspec.LexerRuleset[LangSpecLexerTokenType, LangSpecLexerTokenRole],
) {
	return BuildLexerSpec(spec)
}
