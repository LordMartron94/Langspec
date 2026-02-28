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
		"PROGRAM",
		NodeProgram,
		false,
		parseHeader(ruleBuilder),
		parseBody(ruleBuilder),
		ruleBuilder.Token.ExpectVirtual("EOF", TokEOF),
	)
}

func parseHeader(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Nest(
		"HEADER",
		NodeHeader,
		TokDashes, TokDashes, // open + close
		ruleBuilder.Rule.Sequence(
			"HEAEDER CONTENT",
			NodeHeaderContent,
			ruleBuilder.Token.Expect("DSL NAME", NodeDSLName, TokStringLiteral),
			ruleBuilder.Token.Expect("DSL VERSION", NodeVersion, TokVersion),
			ruleBuilder.Token.ExpectVirtual("HEADER SEPARATOR", TokHeaderSeparator),
			ruleBuilder.Token.ExpectOneOf("LANGSPEC NAME", NodeLSPECName, TokStringLiteral, TokKWLSpec),
			ruleBuilder.Token.Expect("LANGSPEC VERSION", NodeVersion, TokVersion),
		),
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
		ruleBuilder.Token.ExpectOneOf("DECLARE IDENTIFIER", NodeIdentifier, TokKWLexerTokenTypes),
		ruleBuilder.Token.List(
			"DECLARE LIST",
			TokBraceOpen,
			TokStringLiteral, TokComma,
			TokBraceClose,
			NodeList, NodeDeclareIdentifier,
			true, // Allow empty list
			rule.TrailingOptional,
		),
		ruleBuilder.Token.ExpectVirtual("BLOCK CLOSE", TokSemicolon),
	)
}
