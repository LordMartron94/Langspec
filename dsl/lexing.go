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
	TokKWLexerTokenTypes

	TokBracketOpen
	TokBracketClose
	TokSemicolon
	TokComma

	TokComment
)

//go:generate stringer -type LangSpecLexerTokenRole
type LangSpecLexerTokenRole uint8

const (
	LANG_SPEC_STRUCTURAL_ROLE LangSpecLexerTokenRole = iota + 1
	LANG_SPEC_WHITESPACE_ROLE
	LANG_SPEC_COMMENT_ROLE
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

// ===========================================================
// ATOM HELPERS (make grammar list declarative)
// ===========================================================

// --- Base constructors (tiny, boring, reusable)

func def(p pattern.RegulaAST[rune], tok LangSpecLexerTokenType, role LangSpecLexerTokenRole, prio int) ruleDef {
	return ruleDef{pattern: p, token: tok, role: role, priority: prio}
}

func structural(p pattern.RegulaAST[rune], tok LangSpecLexerTokenType, prio int) ruleDef {
	return def(p, tok, LANG_SPEC_STRUCTURAL_ROLE, prio)
}

func trivia(p pattern.RegulaAST[rune], tok LangSpecLexerTokenType, role LangSpecLexerTokenRole, prio int) ruleDef {
	return def(p, tok, role, prio)
}

// --- Structural literals

func litRune(ch rune, tok LangSpecLexerTokenType) ruleDef {
	return structural(pattern.Literal(ch), tok, 0)
}

func litString(s string, tok LangSpecLexerTokenType) ruleDef {
	return structural(pattern.LiteralString[rune](s), tok, 0)
}

// --- Keywords (still structural; priority 1 to beat weaker matches if needed)

func kw(s string, tok LangSpecLexerTokenType) ruleDef {
	return structural(pattern.LiteralString[rune](s), tok, 1)
}

// --- Whitespace / comments (trivia)

func ws() ruleDef {
	ws := pattern.AnyOf(
		pattern.Literal(' '),
		pattern.Literal('\t'),
		pattern.Literal('\n'),
	).Plus()
	return trivia(ws, TokWhitespace, LANG_SPEC_WHITESPACE_ROLE, 0)
}

func commentLine() ruleDef {
	// anything except newline
	notNL := pattern.Class(
		pattern.Range(0, '\n'-1),
		pattern.Range('\n'+1, rune(0x10FFFF)),
	)

	line := pattern.Sequence(
		pattern.Literal('/'),
		pattern.Literal('/'),
		notNL.Star(),
	)
	return trivia(line, TokComment, LANG_SPEC_COMMENT_ROLE, 0)
}

func commentBlock() ruleDef {
	// This is the only mildly annoying part without a "not this sequence" primitive.
	// We'll do a safe but simple variant:
	//   "/*" ( (not '*') | ('*' not '/') )* "*/"
	//
	// That accepts anything until it sees the terminating */.
	notStar := pattern.Class(
		pattern.Range(0, '*'-1),
		pattern.Range('*'+1, rune(0x10FFFF)),
	)
	starNotSlash := pattern.Sequence(
		pattern.Literal('*'),
		pattern.Class(
			pattern.Range(0, '/'-1),
			pattern.Range('/'+1, rune(0x10FFFF)),
		),
	)

	bodyUnit := pattern.AnyOf(notStar, starNotSlash)

	block := pattern.Sequence(
		pattern.Literal('/'),
		pattern.Literal('*'),
		bodyUnit.Star(),
		pattern.Literal('*'),
		pattern.Literal('/'),
	)
	return trivia(block, TokComment, LANG_SPEC_COMMENT_ROLE, 0)
}

// --- Atoms

func stringLiteralNoEsc() ruleDef {
	notQuote := pattern.Class(
		pattern.Range(0, '"'-1),
		pattern.Range('"'+1, rune(0x10FFFF)),
	)
	quoted := pattern.Sequence(
		pattern.Literal('"'),
		notQuote.Star(),
		pattern.Literal('"'),
	)
	return structural(quoted, TokStringLiteral, 1)
}

func versionSemverV3() ruleDef {
	digits := pattern.Digit.Plus()
	version := pattern.Sequence(
		pattern.Literal('v'),
		digits,
		pattern.Literal('.'),
		digits,
		pattern.Literal('.'),
		digits,
	)
	return structural(version, TokVersion, 1)
}

// ===========================================================

func buildLangSpecDSLSpec() (
	*langspec.LangSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind],
	*lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
) {
	lexerSpec, ruleset := buildLangSpecDSLLexerSpec()
	lexerSpec.WithDFADebugFormatter(
		lexarch.LexerDebugFormatterCreateRune[LangSpecLexerState, LangSpecLexerTokenType, LangSpecLexerTokenRole](),
	)

	parserSpec := buildLangSpecDSLParserSpec()

	dslSpec := langspec.LangSpecCreate(
		lexerSpec,
		parserSpec,
	)

	return dslSpec, ruleset
}

func buildLangSpecDSLLexerSpec() (
	*langspec.LexerSpec[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState],
	*lexarch.LexingRuleset[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole],
) {
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
	// GRAMMAR (purely declarative)
	// -------------------------

	addRules(rs,
		// trivia (ignored by parser, but still lexed)
		ws(),
		commentLine(),
		commentBlock(),

		// header / punctuation (priority 0)
		litString("---", TokDashes),
		litRune('|', TokHeaderSeparator),
		litRune(',', TokComma),
		litRune(';', TokSemicolon),
		litRune('{', TokBracketOpen),
		litRune('}', TokBracketClose),

		// atoms (priority 1)
		stringLiteralNoEsc(),
		versionSemverV3(),

		// keywords (priority 1)
		kw("lspec", TokKWLSpec),
		kw("declare", TokKWDeclare),
		kw("LexerTokenTypes", TokKWLexerTokenTypes),
	)

	lexerSpec.WithRuleset(LANG_SPEC_LEXER_STATE_DEFAULT, *rs)
	return lexerSpec, rs
}
