package dsl

import (
	"autarch/pattern"
	"foundation/domain"
	"syntaxa"
)

// ----------------------------------------------------------- SINGLE SOURCE OF TRUTH

/*
BuildLanguageSpec is the only place where language constructs are declared.
Returns a LanguageSpec with tokens (each carrying its pattern directly) and header steps.
No init() or package-level registries; all definition data lives in the returned value.
*/
func BuildLanguageSpec(f *pattern.RegulaASTFactory[rune], t *pattern.RegulaTemplates[rune]) LanguageSpec {
	spec := LanguageSpec{}

	spec.Tokens = []TokenDefinition{
		DefineToken(TokEOF).Scope("meta.eof").Build(),

		DefineToken(TokWhitespace).
			Role(LANG_SPEC_WHITESPACE_ROLE).
			Scope("punctuation.whitespace").
			Pattern(t.Whitespace().Plus()).
			Build(),

		DefineToken(TokLineComment).
			Role(LANG_SPEC_COMMENT_ROLE).
			Scope("comment.line.double-slash").
			Pattern(buildLineCommentPattern(f)).
			Build(),

		DefineToken(TokBlockComment).
			Role(LANG_SPEC_COMMENT_ROLE).
			Scope("comment.block").
			Pattern(buildBlockCommentPattern(f)).
			Build(),

		DefineToken(TokDashes).
			Scope("punctuation.definition.separator").
			Pattern(pattern.LiteralString(f, "---")).
			Build(),

		DefineToken(TokHeaderSeparator).
			Scope("punctuation.section.header").
			Pattern(f.Literal('|')).
			Build(),

		DefineToken(TokComma).
			Scope("punctuation.separator.comma").
			Pattern(f.Literal(',')).
			Build(),

		DefineToken(TokSemicolon).
			Scope("punctuation.terminator.statement").
			Pattern(f.Literal(';')).
			Build(),

		DefineToken(TokBraceOpen).
			Scope("punctuation.section.braces.begin").
			Pattern(f.Literal('{')).
			Build(),

		DefineToken(TokBraceClose).
			Scope("punctuation.section.braces.end").
			Pattern(f.Literal('}')).
			Build(),

		DefineToken(TokStringLiteral).
			Scope("string.quoted.double").
			HighPriority().
			Pattern(buildStringLiteralPattern(f)).
			Build(),

		DefineToken(TokVersion).
			Scope("constant.numeric.version").
			HighPriority().
			Pattern(buildVersionSemverV3Pattern(f, t)).
			Build(),

		DefineToken(TokKWLSpec).
			Scope("keyword.declaration.lspec").
			HighPriority().
			Pattern(pattern.LiteralString(f, "lspec")).
			Build(),
	}

	spec.Headers = []DSLHeaderNestStep{
		defHeaderStep("expect_name", "meta.block.header",
			Expect(GrammarIDDSLName).
				Node(NodeDSLName).
				Tokens(TokStringLiteral).
				PushNext().
				ScopeOverride("entity.name.language").
				Build(),
		),
		defHeaderStep("expect_version", "",
			Expect(GrammarIDDSLVersion).
				Node(NodeVersion).
				Tokens(TokVersion).
				PushNext().
				Build(),
		),
		defHeaderStep("expect_tail", "",
			Expect(GrammarIDHeaderDashes).
				Tokens(TokDashes).
				Virtual().
				Pop(3).
				NestOnly().
				Build(),

			Expect(GrammarIDHeaderSeparator).
				Tokens(TokHeaderSeparator).
				Virtual().
				Build(),

			Expect(GrammarIDLangspecName).
				Node(NodeLSPECName).
				Tokens(TokStringLiteral, TokKWLSpec).
				Build(),

			Expect(GrammarIDLangspecVersion).
				Node(NodeVersion).
				Tokens(TokVersion).
				Build(),
		),
	}

	return spec
}

/*
DSLSpecFactoryAndTemplates creates the factory and templates used by BuildLanguageSpec.
Exported so the compiler or spec_builder can create them without depending on package-level state.
*/
func DSLSpecFactoryAndTemplates() (*pattern.RegulaASTFactory[rune], *pattern.RegulaTemplates[rune]) {
	f := pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())
	t := pattern.RegulaTemplatesCreate(f)
	return f, t
}

// ----------------------------------------------------------- LEXER / PARSER ENUMS (single place for definition)

//go:generate stringer -type LangSpecLexerState
type LangSpecLexerState uint8

const (
	LANG_SPEC_LEXER_STATE_DEFAULT LangSpecLexerState = iota + 1
)

//go:generate stringer -type LangSpecLexerTokenType
type LangSpecLexerTokenType uint32

const (
	TokEOF LangSpecLexerTokenType = iota + 1
	TokWhitespace
	TokDashes
	TokHeaderSeparator
	TokStringLiteral
	TokVersion
	TokKWLSpec
	TokBraceOpen
	TokBraceClose
	TokSemicolon
	TokComma
	TokLineComment
	TokBlockComment
)

//go:generate stringer -type LangSpecLexerTokenRole
type LangSpecLexerTokenRole uint8

const (
	LANG_SPEC_STRUCTURAL_ROLE LangSpecLexerTokenRole = iota + 1
	LANG_SPEC_WHITESPACE_ROLE
	LANG_SPEC_COMMENT_ROLE
)

//go:generate stringer -type LangSpecParserNodeKind
type LangSpecParserNodeKind uint32

const (
	NodeError LangSpecParserNodeKind = iota + 1
	NodeProgram
	NodeHeader
	NodeHeaderContent
	NodeBody
	NodeDSLName
	NodeVersion
	NodeLSPECName
	NodeIdentifier
)

const (
	GrammarIDProgram           syntaxa.GrammarID = "PROGRAM"
	GrammarIDHeader            syntaxa.GrammarID = "HEADER"
	GrammarIDHeaderContent     syntaxa.GrammarID = "HEADER CONTENT"
	GrammarIDDeclarationBlocks syntaxa.GrammarID = "DECLARATION BLOCKS"
	GrammarIDDeclarationBlock  syntaxa.GrammarID = "DECLARATION BLOCK"
	GrammarIDDeclareList       syntaxa.GrammarID = "DECLARE LIST"
	GrammarIDEOF               syntaxa.GrammarID = "EOF"
	GrammarIDHeaderSeparator   syntaxa.GrammarID = "HEADER SEPARATOR"
	GrammarIDHeaderDashes      syntaxa.GrammarID = "HEADER DASHES"
	GrammarIDDSLName           syntaxa.GrammarID = "DSL NAME"
	GrammarIDDSLVersion        syntaxa.GrammarID = "DSL VERSION"
	GrammarIDLangspecName      syntaxa.GrammarID = "LANGSPEC NAME"
	GrammarIDLangspecVersion   syntaxa.GrammarID = "LANGSPEC VERSION"
	GrammarIDDeclareKeyword    syntaxa.GrammarID = "DECLARE KEYWORD"
	GrammarIDDeclareIdentifier syntaxa.GrammarID = "DECLARE IDENTIFIER"
	GrammarIDBlockClose        syntaxa.GrammarID = "BLOCK CLOSE"
)

// ----------------------------------------------------------- PATTERN HELPERS (used only by BuildLanguageSpec)

func buildLineCommentPattern(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	notTerminator := f.NegatedClass(
		f.Range('\n', '\n'),
		f.Range('\r', '\r'),
	)
	return f.Sequence(
		f.Literal('/'),
		f.Literal('/'),
		notTerminator.Star(),
	)
}

func buildBlockCommentPattern(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	notStar := f.NegatedClass(f.Range('*', '*'))
	notStarNorSlash := f.NegatedClass(
		f.Range('*', '*'),
		f.Range('/', '/'),
	)
	stars := f.Literal('*').Plus()
	starPlusNotSlash := f.Sequence(stars, notStarNorSlash)
	bodyUnit := f.AnyOf(notStar, starPlusNotSlash).Star()
	return f.Sequence(
		f.Literal('/'),
		f.Literal('*'),
		bodyUnit,
		stars,
		f.Literal('/'),
	)
}

func buildStringLiteralPattern(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	normalChar := f.NegatedClass(
		f.Range('"', '"'),
		f.Range('\\', '\\'),
		f.Range('\n', '\n'),
		f.Range('\r', '\r'),
	)
	escapeTrigger := f.Literal('\\')
	escapedChar := f.Class(
		f.Range('"', '"'),
		f.Range('\\', '\\'),
		f.Range('n', 'n'),
		f.Range('r', 'r'),
		f.Range('t', 't'),
	)
	escapeSequence := f.Sequence(escapeTrigger, escapedChar)
	body := f.AnyOf(normalChar, escapeSequence).Star()
	quote := f.Literal('"')
	return f.Sequence(quote, body, quote)
}

func buildVersionSemverV3Pattern(f *pattern.RegulaASTFactory[rune], t *pattern.RegulaTemplates[rune]) pattern.RegulaAST[rune] {
	digits := t.Digit().Plus()
	return f.Sequence(
		f.Literal('v'),
		digits,
		f.Literal('.'),
		digits,
		f.Literal('.'),
		digits,
	)
}
