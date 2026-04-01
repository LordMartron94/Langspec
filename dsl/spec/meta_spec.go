package spec

import (
	"langspec"
	"lexarch"
	"syntaxa"
	"syntaxa/rule"
)

/*
AdditionalRulesFromBuilder returns all defined context-boundary grammars from the builder
except the root, for use as ProducePackage additionalRules (disconnected sub-graphs).
*/
func AdditionalRulesFromBuilder(
	rb *rule.RuleBuilder[LangSpecParserNodeKind],
	root *syntaxa.Grammar[lexarch.TokenKind, LangSpecParserNodeKind],
) []*syntaxa.Grammar[lexarch.TokenKind, LangSpecParserNodeKind] {
	defined := rb.GetDefinedGrammars()
	out := make([]*syntaxa.Grammar[lexarch.TokenKind, LangSpecParserNodeKind], 0, len(defined))
	for _, g := range defined {
		if g != root {
			out = append(out, g)
		}
	}
	return out
}

/*
BuildLangSpecDSLSpec builds the LanguageSpec, LangParser configuration, default lexing ruleset,
and program rule for the LangSpec meta-language (.lspec).
*/
func BuildLangSpecDSLSpec() (
	LanguageSpec,
	*langspec.LangSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	*langspec.LexerRuleset[LangSpecLexerTokenType, LangSpecLexerTokenRole],
	Rule,
) {
	factory, templates := dslSpecFactoryAndTemplates()
	languageSpec := buildLanguageSpec(factory, templates)

	lexerSpec, ruleset := buildLangSpecDSLLexerSpec(languageSpec)
	ruleBuilder := rule.RuleBuilderCreate[LangSpecParserNodeKind](
		func(token lexarch.TokenKind) string { return LangSpecLexerTokenType(token).String() },
	)
	g := grammarDefinerCreate(ruleBuilder)
	programRule := buildProgramRule(g)
	registry := ruleBuilder.GetRegistry()
	additionalRules := AdditionalRulesFromBuilder(ruleBuilder, programRule.GetGrammar())

	parserSpec, _ := buildLangSpecDSLParserSpec(programRule, registry, additionalRules)

	dslSpec := langspec.LangSpecCreate(
		lexerSpec,
		parserSpec,
	)

	return languageSpec, dslSpec, ruleset, programRule
}
