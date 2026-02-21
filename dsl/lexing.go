package dsl

import (
	"autarch/pattern"
	"langspec"
	"lexarch"
)

// ----------------------------------------------------------- TYPES

type LangSpecLexerState uint8

const (
	LANG_SPEC_LEXER_STATE_DEFAULT LangSpecLexerState = iota + 1
)

//go:generate stringer -type LangSpecLexerTokenType
type LangSpecLexerTokenType uint32

const (
	// Core
	LANG_SPEC_LEXER_EOF_TOKEN LangSpecLexerTokenType = iota + 1

	LANG_SPEC_LEXER_WHITESPACE

	// Header structure
	LANG_SPEC_LEXER_HEADER_DASHES
	LANG_SPEC_LEXER_HEADER_SEPARATOR

	LANG_SPEC_LEXER_STRING_LITERAL
	LANG_SPEC_LEXER_VERSION

	LANG_SPEC_LEXER_KW_LSPEC

	LANG_SPEC_LEXER_KW_DECLARE
	LANG_SPEC_LEXER_KW_LEXER_STATES
	LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES

	LANG_SPEC_LEXER_BRACKET_OPEN
	LANG_SPEC_LEXER_BRACKET_CLOSE
	LANG_SPEC_LEXER_SEMICOLON
	LANG_SPEC_LEXER_COMMA
)

//go:generate stringer -type LangSpecLexerTokenRole
type LangSpecLexerTokenRole uint8

const (
	LANG_SPEC_STRUCTURAL_ROLE LangSpecLexerTokenRole = iota + 1
	LANG_SPEC_IGNORED_ROLE
	LANG_SPEC_WHITESPACE_ROLE
)

// ----------------------------------------------------------- BUILDING

type ruleDef struct {
	pattern  pattern.RegulaAST[rune]
	token    LangSpecLexerTokenType
	role     LangSpecLexerTokenRole
	priority int
}

func addRules(
	rs *lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
	defs ...ruleDef,
) {
	for _, d := range defs {
		rs.WithRulePriority(d.pattern, d.token, d.role, d.priority)
	}
}

func buildLangSpecDSLSpec() *langspec.LangSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind] {
	lexerSpec := buildLangSpecDSLLexerSpec()
	lexerSpec.WithDFADebugFormatter(
		lexarch.LexerDebugFormatterCreateRune[LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole](),
	)

	parserSpec := buildLangSpecDSLParserSpec()

	dslSpec := langspec.LangSpecCreate(
		lexerSpec,
		parserSpec,
	)

	return dslSpec
}

func buildLangSpecDSLLexerSpec() *langspec.LexerSpec[
	rune,
	LangSpecLexerTokenType,
	LangSpecLexerTokenRole,
	LangSpecLexerState,
] {
	lexerSpec := langspec.LexerSpecCreate[
		rune,
		LangSpecLexerTokenType,
		LangSpecLexerTokenRole](
		LANG_SPEC_LEXER_EOF_TOKEN,
		LANG_SPEC_LEXER_STATE_DEFAULT,
		lexarch.NewlineDetectorRune(),
		lexarch.ColumnAdvanceRune(4),
		runeFormatter,
		lexarch.LexarchRuneSuccessorFn(),
		func(t LangSpecLexerTokenType) string { return t.String() },
	)

	rs := lexarch.LexingRulesetCreate[
		rune,
		LangSpecLexerTokenType,
		LangSpecLexerTokenRole,
	](
		lexarch.TokenResolutionStepLongestThenPriority[LangSpecLexerTokenType],
	)

	// -------------------------
	// GRAMMAR (at a glance)
	// -------------------------

	// whitespace (ignored)
	ws := pattern.AnyOf(
		pattern.Literal(' '),
		pattern.Literal('\t'),
		pattern.Literal('\n'),
	).Plus()
	rs.WithRule(ws, LANG_SPEC_LEXER_WHITESPACE, LANG_SPEC_WHITESPACE_ROLE)

	// string literal: " ... " (no escapes)
	notQuote := pattern.Class(
		pattern.Range(0, '"'-1),
		pattern.Range('"'+1, rune(0x10FFFF)),
	)
	quoted := pattern.Sequence(
		pattern.Literal('"'),
		notQuote.Star(),
		pattern.Literal('"'),
	)

	// version: v1.2.3
	digits := pattern.Digit.Plus()
	version := pattern.Sequence(
		pattern.Literal('v'),
		digits,
		pattern.Literal('.'),
		digits,
		pattern.Literal('.'),
		digits,
	)

	addRules(rs,
		// header / punctuation (priority 0)
		ruleDef{pattern.LiteralString[rune]("---"), LANG_SPEC_LEXER_HEADER_DASHES, LANG_SPEC_STRUCTURAL_ROLE, 0},
		ruleDef{pattern.Literal('|'), LANG_SPEC_LEXER_HEADER_SEPARATOR, LANG_SPEC_STRUCTURAL_ROLE, 0},
		ruleDef{pattern.Literal(','), LANG_SPEC_LEXER_COMMA, LANG_SPEC_STRUCTURAL_ROLE, 0},
		ruleDef{pattern.Literal(';'), LANG_SPEC_LEXER_SEMICOLON, LANG_SPEC_STRUCTURAL_ROLE, 0},
		ruleDef{pattern.Literal('{'), LANG_SPEC_LEXER_BRACKET_OPEN, LANG_SPEC_STRUCTURAL_ROLE, 0},
		ruleDef{pattern.Literal('}'), LANG_SPEC_LEXER_BRACKET_CLOSE, LANG_SPEC_STRUCTURAL_ROLE, 0},

		// atoms (priority 1)
		ruleDef{quoted, LANG_SPEC_LEXER_STRING_LITERAL, LANG_SPEC_STRUCTURAL_ROLE, 1},
		ruleDef{version, LANG_SPEC_LEXER_VERSION, LANG_SPEC_STRUCTURAL_ROLE, 1},

		// keywords (priority 1)
		ruleDef{pattern.LiteralString[rune]("lspec"), LANG_SPEC_LEXER_KW_LSPEC, LANG_SPEC_STRUCTURAL_ROLE, 1},
		ruleDef{pattern.LiteralString[rune]("declare"), LANG_SPEC_LEXER_KW_DECLARE, LANG_SPEC_STRUCTURAL_ROLE, 1},
		ruleDef{pattern.LiteralString[rune]("LexerStates"), LANG_SPEC_LEXER_KW_LEXER_STATES, LANG_SPEC_STRUCTURAL_ROLE, 1},
		ruleDef{pattern.LiteralString[rune]("LexerTokenTypes"), LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES, LANG_SPEC_STRUCTURAL_ROLE, 1},
	)

	lexerSpec.WithRuleset(LANG_SPEC_LEXER_STATE_DEFAULT, *rs)
	return lexerSpec
}
