package dsl

import (
	"langspec"
	"lexarch"
)

func getSession(compiler *LangSpecCompiler, sourceFile string) *langspec.LangParserSession[rune] {
	if compiler.sessionCache != nil {
		compiler.sessionCache.Reset(sourceFile, nil, false)
		return compiler.sessionCache
	}
	session := langspec.LangParserSessionCreate[rune](sourceFile, nil, false)
	compiler.sessionCache = session
	return session
}

func buildLangSpecDSLSpec() (
	*langspec.LangSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	*lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
	Rule,
) {
	lexerSpec, ruleset := buildLangSpecDSLLexerSpec()
	lexerSpec.WithDFADebugFormatter(
		lexarch.LexerDebugFormatterCreateRune[LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole](),
	)

	parserSpec, programRule := buildLangSpecDSLParserSpec()

	dslSpec := langspec.LangSpecCreate(
		lexerSpec,
		parserSpec,
	)

	return dslSpec, ruleset, programRule
}
