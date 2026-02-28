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
	LanguageSpec,
	*langspec.LangSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	*lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
	Rule,
) {
	factory, templates := DSLSpecFactoryAndTemplates()
	spec := BuildLanguageSpec(factory, templates)

	lexerSpec, ruleset := buildLangSpecDSLLexerSpec(spec)
	lexerSpec.WithDFADebugFormatter(
		lexarch.LexerDebugFormatterCreateRune[LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole](),
	)

	parserSpec, programRule := buildLangSpecDSLParserSpec(spec)

	dslSpec := langspec.LangSpecCreate(
		lexerSpec,
		parserSpec,
	)

	return spec, dslSpec, ruleset, programRule
}
