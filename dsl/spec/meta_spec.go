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
	rb *rule.RuleBuilder[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	root *syntaxa.Grammar[LangSpecLexerTokenType, LangSpecParserNodeKind],
) []*syntaxa.Grammar[LangSpecLexerTokenType, LangSpecParserNodeKind] {
	defined := rb.GetDefinedGrammars()
	out := make([]*syntaxa.Grammar[LangSpecLexerTokenType, LangSpecParserNodeKind], 0, len(defined))
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
	*lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
	Rule,
) {
	factory, templates := dslSpecFactoryAndTemplates()
	languageSpec := buildLanguageSpec(factory, templates)

	lexerSpec, ruleset := buildLangSpecDSLLexerSpec(languageSpec)
	lexerSpec.WithDFADebugFormatter(
		lexarch.LexerDebugFormatterCreateRune[LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole](),
	)

	ruleBuilder := rule.RuleBuilderCreate[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind](
		LangSpecLexerTokenType.String,
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
