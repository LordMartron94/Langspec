package dsl

import (
	"langspec"
	"lexarch"
	"syntaxa"
	"syntaxa/rule"
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

/*
additionalRulesFromBuilder returns all defined context-boundary grammars from the builder
except the root, for use as ProducePackage additionalRules (disconnected sub-graphs).
*/
func additionalRulesFromBuilder(
	rb *rule.RuleBuilder[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	root *syntaxa.Grammar[LangSpecLexerTokenType],
) []*syntaxa.Grammar[LangSpecLexerTokenType] {
	defined := rb.GetDefinedGrammars()
	out := make([]*syntaxa.Grammar[LangSpecLexerTokenType], 0, len(defined))
	for _, g := range defined {
		if g != root {
			out = append(out, g)
		}
	}
	return out
}

func buildLangSpecDSLSpec() (
	LanguageSpec,
	*langspec.LangSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	*lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
	Rule,
) {
	factory, templates := dslSpecFactoryAndTemplates()
	spec := buildLanguageSpec(factory, templates)

	lexerSpec, ruleset := buildLangSpecDSLLexerSpec(spec)
	lexerSpec.WithDFADebugFormatter(
		lexarch.LexerDebugFormatterCreateRune[LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole](),
	)

	ruleBuilder := rule.RuleBuilderCreate[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind](
		LangSpecLexerTokenType.String,
	)
	g := grammarDefinerCreate(ruleBuilder)
	programRule := buildProgramRule(g)
	registry := ruleBuilder.GetRegistry()
	additionalRules := additionalRulesFromBuilder(ruleBuilder, programRule.GetGrammar())

	parserSpec, _ := buildLangSpecDSLParserSpec(programRule, registry, additionalRules)

	dslSpec := langspec.LangSpecCreate(
		lexerSpec,
		parserSpec,
	)

	return spec, dslSpec, ruleset, programRule
}
