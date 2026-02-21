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
	LANG_SPEC_ERROR_NODE LangSpecParserNodeKind = iota + 1
	LANG_SPEC_PROGRAM_NODE

	LANG_SPEC_HEADER_NODE
	LANG_SPEC_DECLARATION_BLOCK_NODE

	LANG_SPEC_DSL_NAME_NODE
	LANG_SPEC_VERSION_NODE
	LANG_SPEC_LSPEC_NAME_NODE

	LANG_SPEC_DECLARE_IDENTIFIER_NODE
	LANG_SPEC_IDENTIFIER_NODE
	LANG_SPEC_LIST_NODE

	LANG_SPEC_BODY_NODE
	LANG_SPEC_DECLARATION_BLOCKS_NODE
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
			if lexeme.Token == LANG_SPEC_LEXER_STRING_LITERAL {
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
		LANG_SPEC_PROGRAM_NODE,
		LANG_SPEC_ERROR_NODE,
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
		LANG_SPEC_PROGRAM_NODE,
		false,
		parseHeader(ruleBuilder),
		parseBody(ruleBuilder),
		ruleBuilder.Token.ExpectVirtual("EOF", LANG_SPEC_LEXER_EOF_TOKEN),
	)
}

func parseHeader(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Sequence(
		"HEADER",
		LANG_SPEC_HEADER_NODE,
		ruleBuilder.Token.ExpectVirtual("HEADER START", LANG_SPEC_LEXER_HEADER_DASHES),
		ruleBuilder.Token.Expect("DSL NAME", LANG_SPEC_DSL_NAME_NODE, LANG_SPEC_LEXER_STRING_LITERAL),
		ruleBuilder.Token.Expect("DSL VERSION", LANG_SPEC_VERSION_NODE, LANG_SPEC_LEXER_VERSION),
		ruleBuilder.Token.ExpectVirtual("HEADER SEPARATOR", LANG_SPEC_LEXER_HEADER_SEPARATOR),
		ruleBuilder.Token.ExpectOneOf("LANGSPEC NAME", LANG_SPEC_LSPEC_NAME_NODE, LANG_SPEC_LEXER_STRING_LITERAL, LANG_SPEC_LEXER_KW_LSPEC),
		ruleBuilder.Token.Expect("LANGSPEC VERSION", LANG_SPEC_VERSION_NODE, LANG_SPEC_LEXER_VERSION),
		ruleBuilder.Token.ExpectVirtual("HEADER END", LANG_SPEC_LEXER_HEADER_DASHES),
	)
}

func parseBody(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.ZeroOrMore("DECLARATION BLOCKS", LANG_SPEC_DECLARATION_BLOCKS_NODE, parseDeclarationBlock(ruleBuilder))
}

func parseDeclarationBlock(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.Block(
		"DECLARATION BLOCK",
		LANG_SPEC_DECLARATION_BLOCK_NODE,
		LANG_SPEC_LEXER_SEMICOLON,
		ruleBuilder.Token.ExpectVirtual("DECLARE KEYWORD", LANG_SPEC_LEXER_KW_DECLARE),
		ruleBuilder.Token.ExpectOneOf("DECLARE IDENTIFIER", LANG_SPEC_IDENTIFIER_NODE, LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES, LANG_SPEC_LEXER_KW_LEXER_STATES),
		ruleBuilder.Token.List(
			"DECLARE LIST",
			LANG_SPEC_LEXER_BRACKET_OPEN,
			LANG_SPEC_LEXER_STRING_LITERAL, LANG_SPEC_LEXER_COMMA,
			LANG_SPEC_LEXER_BRACKET_CLOSE,
			LANG_SPEC_LIST_NODE, LANG_SPEC_DECLARE_IDENTIFIER_NODE,
			true, // Allow empty list
			rule.TrailingOptional,
		),
		ruleBuilder.Token.ExpectVirtual("BLOCK CLOSE", LANG_SPEC_LEXER_SEMICOLON),
	)
}
