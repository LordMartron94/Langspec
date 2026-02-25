package dsl

import (
	"autarch/pattern"
	"cmp"
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

	TokBraceOpen
	TokBraceClose
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

var factory = pattern.RegulaASTFactoryCreate(
	func(a, b rune) int {
		return cmp.Compare(a, b)
	},
)

var templates = pattern.RegulaTemplatesCreate(factory)

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
	return structural(factory.Literal(ch), tok, 0)
}

func litString(s string, tok LangSpecLexerTokenType) ruleDef {
	return structural(pattern.LiteralString(factory, s), tok, 0)
}

// --- Keywords (still structural; priority 1 to beat weaker matches if needed)

func kw(s string, tok LangSpecLexerTokenType) ruleDef {
	return structural(pattern.LiteralString(factory, s), tok, 1)
}

// --- Whitespace / comments (trivia)

func ws() ruleDef {
	ws := templates.Whitespace().Plus()
	return trivia(ws, TokWhitespace, LANG_SPEC_WHITESPACE_ROLE, 0)
}

func commentLine() ruleDef {
	// anything except newline
	notNL := factory.Class(
		factory.Range(0, '\n'-1),
		factory.Range('\n'+1, rune(0x10FFFF)),
	)

	line := factory.Sequence(
		factory.Literal('/'),
		factory.Literal('/'),
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
	notStar := factory.Class(
		factory.Range(0, '*'-1),
		factory.Range('*'+1, rune(0x10FFFF)),
	)
	starNotSlash := factory.Sequence(
		factory.Literal('*'),
		factory.Class(
			factory.Range(0, '/'-1),
			factory.Range('/'+1, rune(0x10FFFF)),
		),
	)

	bodyUnit := factory.AnyOf(notStar, starNotSlash)

	block := factory.Sequence(
		factory.Literal('/'),
		factory.Literal('*'),
		bodyUnit.Star(),
		factory.Literal('*'),
		factory.Literal('/'),
	)
	return trivia(block, TokComment, LANG_SPEC_COMMENT_ROLE, 0)
}

// --- Atoms

func stringLiteralNoEsc() ruleDef {
	notQuote := factory.Class(
		factory.Range(0, '"'-1),
		factory.Range('"'+1, rune(0x10FFFF)),
	)
	quoted := factory.Sequence(
		factory.Literal('"'),
		notQuote.Star(),
		factory.Literal('"'),
	)
	return structural(quoted, TokStringLiteral, 1)
}

func versionSemverV3() ruleDef {
	digits := templates.Digit().Plus()
	version := factory.Sequence(
		factory.Literal('v'),
		digits,
		factory.Literal('.'),
		digits,
		factory.Literal('.'),
		digits,
	)
	return structural(version, TokVersion, 1)
}

// ===========================================================

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
		lexarch.RunesToBytesDefault(),
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
		litRune('{', TokBraceOpen),
		litRune('}', TokBraceClose),

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
