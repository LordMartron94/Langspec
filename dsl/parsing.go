package dsl

import (
	"langspec"
	"strings"
	"syntaxa"
	"syntaxa/rule"
)

// --------------------------------------------------------------- ATTRIBUTES (node kinds live in constructs.go)

const (
	ATTRIBUTE_LITERAL_STRING_FORMATTED = "formattedString"
	ATTRIBUTE_RULE_NAME                = "ruleName"
)

// --------------------------------------------------------------- BUILDING

func buildLangSpecDSLParserSpec(spec LanguageSpec) (
	*langspec.ParserSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	Rule,
) {
	finalizationPostProcessor := func(node *Node, finalizationCTX *NodeFinalizationCtx, ruleIdentity syntaxa.RuleIdentity) {
		// fmt.Printf("Post processing node created by: %s\n", ruleIdentity.RuleName)
		finalizationCTX.SetAttribute(node, ATTRIBUTE_RULE_NAME, ruleIdentity.RuleName)

		lexemes := node.Tokens()
		for _, lexeme := range lexemes {
			if lexeme.Token == TokStringLiteral {
				raw := node.GetContent(" ")
				formatted := strings.Replace(raw, "\"", "", -1)
				finalizationCTX.SetAttribute(node, ATTRIBUTE_LITERAL_STRING_FORMATTED, formatted)
				break
			}
		}
	}

	ruleBuilder := rule.RuleBuilderCreate[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind](
		LangSpecLexerTokenType.String,
	)

	programRule := parseProgram(ruleBuilder, spec)

	parserSpec := langspec.ParserSpecCreate(
		NodeProgram,
		NodeError,
		programRule,
		true, // freeze AST after parse
	)
	parserSpec.WithSkipRoles(
		LANG_SPEC_WHITESPACE_ROLE,
		LANG_SPEC_COMMENT_ROLE,
	)
	parserSpec.WithPostProcessor(finalizationPostProcessor)

	return parserSpec, programRule
}

func parseProgram(ruleBuilder *RuleBuilder, spec LanguageSpec) Rule {
	return ruleBuilder.Rule.Root(
		GrammarIDProgram,
		NodeProgram,
		false,
		parseHeader(ruleBuilder, spec),
		// parseBody(ruleBuilder),
		ruleBuilder.Token.ExpectVirtual(GrammarIDEOF, TokEOF),
	)
}

func parseHeader(ruleBuilder *RuleBuilder, spec LanguageSpec) Rule {
	flat := LanguageSpecHeaderExpectationsFlat(spec)
	sequenceRules := make([]Rule, 0, len(flat))
	for _, e := range flat {
		if e.Virtual {
			if len(e.Tokens) > 0 {
				sequenceRules = append(sequenceRules, ruleBuilder.Token.ExpectVirtual(e.GrammarID, e.Tokens[0]))
			}
			continue
		}
		if len(e.Tokens) == 1 {
			sequenceRules = append(sequenceRules, ruleBuilder.Token.Expect(e.GrammarID, e.NodeKind, e.Tokens[0]))
		} else if len(e.Tokens) > 1 {
			sequenceRules = append(sequenceRules, ruleBuilder.Token.ExpectOneOf(e.GrammarID, e.NodeKind, e.Tokens...))
		}
	}
	return ruleBuilder.Rule.Nest(
		GrammarIDHeader,
		NodeHeader,
		TokDashes, TokDashes,
		ruleBuilder.Rule.Sequence(
			GrammarIDHeaderContent,
			NodeHeaderContent,
			sequenceRules...,
		),
	)
}

// func parseBody(ruleBuilder *RuleBuilder) Rule {
// 	return ruleBuilder.Rule.ZeroOrMore(GrammarIDDeclarationBlocks, NodeDeclarationBlocks, parseDeclarationBlock(ruleBuilder))
// }
