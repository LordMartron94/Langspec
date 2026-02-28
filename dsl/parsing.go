package dsl

import (
	"langspec"
	"strings"
	"syntaxa"
	"syntaxa/rule"
)

// --------------------------------------------------------------- TYPES

//go:generate stringer -type LangSpecParserNodeKind
type LangSpecParserNodeKind uint32

const (
	// SPECIAL
	NodeError LangSpecParserNodeKind = iota + 1

	// ROOT
	NodeProgram
	NodeHeader
	NodeHeaderContent
	NodeBody

	// ATOMS
	NodeDSLName
	NodeVersion
	NodeLSPECName

	NodeIdentifier
)

// --------------------------------------------------------------- ATTRIBUTES

const (
	ATTRIBUTE_LITERAL_STRING_FORMATTED = "formattedString"
	ATTRIBUTE_RULE_NAME                = "ruleName"
)

// --------------------------------------------------------------- BUILDING

func buildLangSpecDSLParserSpec() (
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

	programRule := parseProgram(ruleBuilder)

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

func parseProgram(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Root(
		GrammarIDProgram,
		NodeProgram,
		false,
		parseHeader(ruleBuilder),
		// parseBody(ruleBuilder),
		ruleBuilder.Token.ExpectVirtual(GrammarIDEOF, TokEOF),
	)
}

func parseHeader(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Nest(
		GrammarIDHeader,
		NodeHeader,
		TokDashes, TokDashes, // open + close
		ruleBuilder.Rule.Sequence(
			GrammarIDHeaderContent,
			NodeHeaderContent,
			ruleBuilder.Token.Expect(GrammarIDDSLName, NodeDSLName, TokStringLiteral),
			ruleBuilder.Token.Expect(GrammarIDDSLVersion, NodeVersion, TokVersion),
			ruleBuilder.Token.ExpectVirtual(GrammarIDHeaderSeparator, TokHeaderSeparator),
			ruleBuilder.Token.ExpectOneOf(GrammarIDLangspecName, NodeLSPECName, TokStringLiteral, TokKWLSpec),
			ruleBuilder.Token.Expect(GrammarIDLangspecVersion, NodeVersion, TokVersion),
		),
	)
}

// func parseBody(ruleBuilder *RuleBuilder) Rule {
// 	return ruleBuilder.Rule.ZeroOrMore(GrammarIDDeclarationBlocks, NodeDeclarationBlocks, parseDeclarationBlock(ruleBuilder))
// }
