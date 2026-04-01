package spec

import (
	"foundation/text"
	"langspec"
	"lexarch"
	"syntaxa"
	"syntaxa/lowering"
)

const (
	ATTRIBUTE_LITERAL_STRING_VALUE = "stringLiteralValue"
	ATTRIBUTE_CHAR_LITERAL_VALUE   = "charLiteralValue"
	ATTRIBUTE_REGEX_LITERAL_VALUE  = "regexLiteralValue"
)

// --------------------------------------------------------------- BUILDING

func buildLangSpecDSLParserSpec(
	programRule Rule,
	registry syntaxa.RuleRegistry[LangSpecParserNodeKind],
	additionalRules []*syntaxa.Grammar[lexarch.TokenKind, LangSpecParserNodeKind],
) (
	*langspec.ParserSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	Rule,
) {
	grammarPkg := new(syntaxa.GrammarPackage[LangSpecParserNodeKind])
	*grammarPkg = syntaxa.ProducePackage(
		programRule.GetGrammar(),
		additionalRules,
		"LangSpec DSL",
		"0.0.0",
		&programRule,
	)
	getAnalysis := func() *syntaxa.GrammarAnalysis { return lowering.GetAnalysis(grammarPkg) }
	parserSpec := langspec.ParserSpecCreate[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind](
		grammarPkg,
		registry,
		NodeProgram,
		NodeError,
		true, // freeze LST after parse
		getAnalysis,
	)
	parserSpec.WithSkipRoles(
		LANG_SPEC_WHITESPACE_ROLE,
		LANG_SPEC_COMMENT_ROLE,
	)
	parserSpec.WithPostProcessor(applyLiteralAttributes)

	return parserSpec, programRule
}

// applyLiteralAttributes is extracted to keep cognitive load low and isolate AST modification logic.
func applyLiteralAttributes(node *Node, finalizationCTX *NodeFinalizationCtx, _ syntaxa.RuleIdentity) {
	lexemes := node.Tokens()
	if len(lexemes) == 0 {
		return
	}

	token := lexemes[0]
	raw := string(token.Raw)

	switch LangSpecLexerTokenType(token.Token) {
	case TokStringLiteral:
		inner := stripQuotes(raw, '"')
		finalizationCTX.SetAttribute(node, ATTRIBUTE_LITERAL_STRING_VALUE, text.Unescape(inner))

	case TokCharLiteral:
		inner := stripQuotes(raw, '\'')
		finalizationCTX.SetAttribute(node, ATTRIBUTE_CHAR_LITERAL_VALUE, text.Unescape(inner))

	case TokRegexLiteral:
		inner := stripQuotes(raw, '`')
		finalizationCTX.SetAttribute(node, ATTRIBUTE_REGEX_LITERAL_VALUE, inner)
	}
}

// stripQuotes safely removes surrounding quote characters if they match the expected boundary.
func stripQuotes(raw string, boundary byte) string {
	if len(raw) >= 2 && raw[0] == boundary && raw[len(raw)-1] == boundary {
		return raw[1 : len(raw)-1]
	}
	return raw
}
