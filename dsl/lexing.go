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
	TokEOF LangSpecLexerTokenType = iota + 1

	TokWhitespace

	// Header structure
	TokDashes
	TokHeaderSeparator

	TokStringLiteral
	TokVersion

	TokKWLSpec

	TokKWDeclare
	TokKWLexerStates
	TokKWLexerTokenTypes

	TokBracketOpen
	TokBracketClose
	TokSemicolon
	TokComma
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
		TokEOF,
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
	rs.WithRule(ws, TokWhitespace, LANG_SPEC_WHITESPACE_ROLE)

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
		ruleDef{pattern.LiteralString[rune]("---"), TokDashes, LANG_SPEC_STRUCTURAL_ROLE, 0},
		ruleDef{pattern.Literal('|'), TokHeaderSeparator, LANG_SPEC_STRUCTURAL_ROLE, 0},
		ruleDef{pattern.Literal(','), TokComma, LANG_SPEC_STRUCTURAL_ROLE, 0},
		ruleDef{pattern.Literal(';'), TokSemicolon, LANG_SPEC_STRUCTURAL_ROLE, 0},
		ruleDef{pattern.Literal('{'), TokBracketOpen, LANG_SPEC_STRUCTURAL_ROLE, 0},
		ruleDef{pattern.Literal('}'), TokBracketClose, LANG_SPEC_STRUCTURAL_ROLE, 0},

		// atoms (priority 1)
		ruleDef{quoted, TokStringLiteral, LANG_SPEC_STRUCTURAL_ROLE, 1},
		ruleDef{version, TokVersion, LANG_SPEC_STRUCTURAL_ROLE, 1},

		// keywords (priority 1)
		ruleDef{pattern.LiteralString[rune]("lspec"), TokKWLSpec, LANG_SPEC_STRUCTURAL_ROLE, 1},
		ruleDef{pattern.LiteralString[rune]("declare"), TokKWDeclare, LANG_SPEC_STRUCTURAL_ROLE, 1},
		ruleDef{pattern.LiteralString[rune]("LexerStates"), TokKWLexerStates, LANG_SPEC_STRUCTURAL_ROLE, 1},
		ruleDef{pattern.LiteralString[rune]("LexerTokenTypes"), TokKWLexerTokenTypes, LANG_SPEC_STRUCTURAL_ROLE, 1},
	)

	lexerSpec.WithRuleset(LANG_SPEC_LEXER_STATE_DEFAULT, *rs)
	return lexerSpec
}
