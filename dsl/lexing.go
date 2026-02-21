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

func (l LangSpecLexerTokenType) String() string {
	switch l {
	case LANG_SPEC_LEXER_EOF_TOKEN:
		return "EOF"
	case LANG_SPEC_LEXER_WHITESPACE:
		return "WHITESPACE"
	case LANG_SPEC_LEXER_HEADER_DASHES:
		return "DASHES"
	case LANG_SPEC_LEXER_HEADER_SEPARATOR:
		return "HEADER SEPARATOR"
	case LANG_SPEC_LEXER_STRING_LITERAL:
		return "STRING LITERAL"
	case LANG_SPEC_LEXER_VERSION:
		return "VERSION"
	case LANG_SPEC_LEXER_KW_DECLARE:
		return "DECLARE KW"
	case LANG_SPEC_LEXER_KW_LEXER_STATES:
		return "LEXER STATES KW"
	case LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES:
		return "TOKEN TYPES KW"
	case LANG_SPEC_LEXER_BRACKET_OPEN:
		return "BRACKET OPEN"
	case LANG_SPEC_LEXER_BRACKET_CLOSE:
		return "BRACKET CLOSE"
	case LANG_SPEC_LEXER_SEMICOLON:
		return "SEMICOLON"
	case LANG_SPEC_LEXER_KW_LSPEC:
		return "LSPEC KW"
	case LANG_SPEC_LEXER_COMMA:
		return "COMMA"
	default:
		return "UNKNOWN TOKEN TYPE"
	}
}

type LangSpecLexerTokenRole uint8

const (
	LANG_SPEC_STRUCTURAL_ROLE LangSpecLexerTokenRole = iota + 1
	LANG_SPEC_IGNORED_ROLE
	LANG_SPEC_WHITESPACE_ROLE
)

func (l LangSpecLexerTokenRole) String() string {
	switch l {
	case LANG_SPEC_STRUCTURAL_ROLE:
		return "Structural"
	case LANG_SPEC_IGNORED_ROLE:
		return "IGNORED"
	case LANG_SPEC_WHITESPACE_ROLE:
		return "WHITESPACE"
	default:
		return "UNKNOWN TOKEN ROLE"
	}
}

// ----------------------------------------------------------- BUILDING

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
		func(token LangSpecLexerTokenType) string {
			return token.String()
		},
	)

	defaultRuleset := lexarch.LexingRulesetCreate[
		rune,
		LangSpecLexerTokenType,
		LangSpecLexerTokenRole,
	](
		lexarch.TokenResolutionStepLongestThenPriority[LangSpecLexerTokenType],
	)

	// ------------------------------------------------------------
	// Whitespace (ignored)
	// ------------------------------------------------------------

	ws := pattern.AnyOf(
		pattern.Literal(' '),
		pattern.Literal('\t'),
		pattern.Literal('\n'),
	).Plus()

	defaultRuleset.WithRule(ws, LANG_SPEC_LEXER_WHITESPACE, LANG_SPEC_WHITESPACE_ROLE)

	// ------------------------------------------------------------
	// Header structure
	// ------------------------------------------------------------

	defaultRuleset.WithRulePriority(
		pattern.LiteralString[rune]("---"),
		LANG_SPEC_LEXER_HEADER_DASHES,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	defaultRuleset.WithRulePriority(
		pattern.Literal('|'),
		LANG_SPEC_LEXER_HEADER_SEPARATOR,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	defaultRuleset.WithRulePriority(
		pattern.Literal(','),
		LANG_SPEC_LEXER_COMMA,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	defaultRuleset.WithRulePriority(
		pattern.Literal(';'),
		LANG_SPEC_LEXER_SEMICOLON,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	defaultRuleset.WithRulePriority(
		pattern.Literal('{'),
		LANG_SPEC_LEXER_BRACKET_OPEN,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	defaultRuleset.WithRulePriority(
		pattern.Literal('}'),
		LANG_SPEC_LEXER_BRACKET_CLOSE,
		LANG_SPEC_STRUCTURAL_ROLE,
		0,
	)

	// ------------------------------------------------------------
	// Language name: "TEST LANGUAGE"
	// ------------------------------------------------------------

	notQuote := pattern.Class(
		pattern.Range(0, '"'-1),
		pattern.Range('"'+1, rune(0x10FFFF)),
	)

	quoted := pattern.Sequence(
		pattern.Literal('"'),
		notQuote.Star(),
		pattern.Literal('"'),
	)

	defaultRuleset.WithRulePriority(
		quoted,
		LANG_SPEC_LEXER_STRING_LITERAL,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	// ------------------------------------------------------------
	// Versions: v1.0.0
	// ------------------------------------------------------------

	digits := pattern.Digit.Plus()

	version := pattern.Sequence(
		pattern.Literal('v'),
		digits,
		pattern.Literal('.'),
		digits,
		pattern.Literal('.'),
		digits,
	)

	defaultRuleset.WithRulePriority(
		version,
		LANG_SPEC_LEXER_VERSION,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	// ------------------------------------------------------------
	// DSL name: lspec
	// ------------------------------------------------------------

	defaultRuleset.WithRulePriority(
		pattern.LiteralString[rune]("lspec"),
		LANG_SPEC_LEXER_KW_LSPEC,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	defaultRuleset.WithRulePriority(
		pattern.LiteralString[rune]("declare"),
		LANG_SPEC_LEXER_KW_DECLARE,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	defaultRuleset.WithRulePriority(
		pattern.LiteralString[rune]("LexerStates"),
		LANG_SPEC_LEXER_KW_LEXER_STATES,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	defaultRuleset.WithRulePriority(
		pattern.LiteralString[rune]("LexerTokenTypes"),
		LANG_SPEC_LEXER_KW_LEXER_TOKEN_TYPES,
		LANG_SPEC_STRUCTURAL_ROLE,
		1,
	)

	// ------------------------------------------------------------

	lexerSpec.WithRuleset(
		LANG_SPEC_LEXER_STATE_DEFAULT,
		*defaultRuleset,
	)

	return lexerSpec
}
