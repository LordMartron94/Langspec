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
	NodeBody

	// AGGREGATIONS
	NodeDeclarationBlocks
	NodeDeclarationBlock

	NodeList

	// ATOMS
	NodeDSLName
	NodeVersion
	NodeLSPECName

	NodeDeclareIdentifier
	NodeIdentifier
)

// --------------------------------------------------------------- ATTRIBUTES

const (
	ATTRIBUTE_LITERAL_STRING_FORMATTED = "formattedString"
	ATTRIBUTE_RULE_NAME                = "ruleName"
)

// --------------------------------------------------------------- BUILDING

func buildLangSpecDSLParserSpec() *langspec.ParserSpec[
	rune,
	LangSpecLexerTokenType,
	LangSpecLexerTokenRole,
	LangSpecLexerState,
	LangSpecParserNodeKind,
] {
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

	parserSpec := langspec.ParserSpecCreate(
		NodeProgram,
		NodeError,
		parseProgram(ruleBuilder),
		true, // freeze AST after parse
	)
	parserSpec.WithSkipRoles(
		LANG_SPEC_WHITESPACE_ROLE,
		LANG_SPEC_IGNORED_ROLE,
	)
	parserSpec.WithPostProcessor(finalizationPostProcessor)

	return parserSpec
}

func parseProgram(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Root(
		"PROGRAM",
		NodeProgram,
		false,
		parseHeader(ruleBuilder),
		parseBody(ruleBuilder),
		ruleBuilder.Token.ExpectVirtual("EOF", TokEOF),
	)
}

func parseHeader(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Sequence(
		"HEADER",
		NodeHeader,
		ruleBuilder.Token.ExpectVirtual("HEADER START", TokDashes),
		ruleBuilder.Token.Expect("DSL NAME", NodeDSLName, TokStringLiteral),
		ruleBuilder.Token.Expect("DSL VERSION", NodeVersion, TokVersion),
		ruleBuilder.Token.ExpectVirtual("HEADER SEPARATOR", TokHeaderSeparator),
		ruleBuilder.Token.ExpectOneOf("LANGSPEC NAME", NodeLSPECName, TokStringLiteral, TokKWLSpec),
		ruleBuilder.Token.Expect("LANGSPEC VERSION", NodeVersion, TokVersion),
		ruleBuilder.Token.ExpectVirtual("HEADER END", TokDashes),
	)
}

func parseBody(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.ZeroOrMore("DECLARATION BLOCKS", NodeDeclarationBlocks, parseDeclarationBlock(ruleBuilder))
}

func parseDeclarationBlock(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Block(
		"DECLARATION BLOCK",
		NodeDeclarationBlock,
		TokSemicolon,
		ruleBuilder.Token.ExpectVirtual("DECLARE KEYWORD", TokKWDeclare),
		ruleBuilder.Token.ExpectOneOf("DECLARE IDENTIFIER", NodeIdentifier, TokKWLexerTokenTypes, TokKWLexerStates),
		ruleBuilder.Token.List(
			"DECLARE LIST",
			TokBracketOpen,
			TokStringLiteral, TokComma,
			TokBracketClose,
			NodeList, NodeDeclareIdentifier,
			true, // Allow empty list
			rule.TrailingOptional,
		),
		ruleBuilder.Token.ExpectVirtual("BLOCK CLOSE", TokSemicolon),
	)
}
