package dsl

import (
	"langspec"
	"lexarch"
)

/*
BuildLexerSpec creates the LexerSpec and LexingRuleset from the language spec.
Iterates spec.Tokens and adds a rule for each token with Pattern != nil.
*/
func BuildLexerSpec(spec LanguageSpec) (
	*langspec.LexerSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState],
	*lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
) {
	lexerSpec := langspec.LexerSpecCreate[
		rune,
		LangSpecLexerTokenType,
		LangSpecLexerTokenRole](
		TokEOF,
		LANG_SPEC_LEXER_STATE_DEFAULT,
		lexarch.NewlineDetectorRune(),
		lexarch.ColumnAdvanceRune(4),
		lexarch.RunesToBytesDefault(),
		runeFormatter,
		lexarch.LexarchRuneDomain(),
		func(t LangSpecLexerTokenType) string { return t.String() },
	)

	rs := lexarch.LexingRulesetCreate[
		rune,
		LangSpecLexerTokenType,
		LangSpecLexerTokenRole,
	](
		lexarch.TokenResolutionStepLongestThenPriority[LangSpecLexerTokenType],
	)

	for _, tok := range spec.Tokens {
		if tok.Pattern != nil {
			rs.WithRulePriority(*tok.Pattern, tok.Type, tok.Role, tok.Priority)
		}
	}

	lexerSpec.WithRuleset(LANG_SPEC_LEXER_STATE_DEFAULT, *rs)
	return lexerSpec, rs
}

func buildLangSpecDSLLexerSpec(spec LanguageSpec) (
	*langspec.LexerSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState],
	*lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
) {
	return BuildLexerSpec(spec)
}
