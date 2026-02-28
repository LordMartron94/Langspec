package dsl

import (
	"autarch/pattern"
	"foundation/domain"
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

func tokenDef(typ LangSpecLexerTokenType, role LangSpecLexerTokenRole, scope string, priority int, p *pattern.RegulaAST[rune]) TokenDefinition {
	return TokenDefinition{
		Type:     typ,
		Role:     role,
		Scope:    scope,
		Priority: priority,
		Pattern:  p,
	}
}

func allocPattern(p pattern.RegulaAST[rune]) *pattern.RegulaAST[rune] {
	q := new(pattern.RegulaAST[rune])
	*q = p
	return q
}

// ----------------------------------------------------------- SINGLE SOURCE OF TRUTH

/*
BuildLanguageSpec is the only place where language constructs are declared.
Returns a LanguageSpec with tokens (each carrying its pattern directly) and header steps.
No init() or package-level registries; all definition data lives in the returned value.
*/
func BuildLanguageSpec(f *pattern.RegulaASTFactory[rune], t *pattern.RegulaTemplates[rune]) LanguageSpec {
	spec := LanguageSpec{}

	// Tokens: trivia first, then structural (order matters for lexer)
	spec.Tokens = []TokenDefinition{
		tokenDef(TokEOF, LANG_SPEC_STRUCTURAL_ROLE, "meta.eof", 0, nil),
		tokenDef(TokWhitespace, LANG_SPEC_WHITESPACE_ROLE, "punctuation.whitespace", 0, allocPattern(t.Whitespace().Plus())),
		tokenDef(TokLineComment, LANG_SPEC_COMMENT_ROLE, "comment.line.double-slash", 0, allocPattern(buildLineCommentPattern(f))),
		tokenDef(TokBlockComment, LANG_SPEC_COMMENT_ROLE, "comment.block", 0, allocPattern(buildBlockCommentPattern(f))),
		tokenDef(TokDashes, LANG_SPEC_STRUCTURAL_ROLE, "punctuation.definition.separator", 0, allocPattern(pattern.LiteralString(f, "---"))),
		tokenDef(TokHeaderSeparator, LANG_SPEC_STRUCTURAL_ROLE, "punctuation.section.header", 0, allocPattern(f.Literal('|'))),
		tokenDef(TokComma, LANG_SPEC_STRUCTURAL_ROLE, "punctuation.separator.comma", 0, allocPattern(f.Literal(','))),
		tokenDef(TokSemicolon, LANG_SPEC_STRUCTURAL_ROLE, "punctuation.terminator.statement", 0, allocPattern(f.Literal(';'))),
		tokenDef(TokBraceOpen, LANG_SPEC_STRUCTURAL_ROLE, "punctuation.section.braces.begin", 0, allocPattern(f.Literal('{'))),
		tokenDef(TokBraceClose, LANG_SPEC_STRUCTURAL_ROLE, "punctuation.section.braces.end", 0, allocPattern(f.Literal('}'))),
		tokenDef(TokStringLiteral, LANG_SPEC_STRUCTURAL_ROLE, "string.quoted.double", 1, allocPattern(buildStringLiteralPattern(f))),
		tokenDef(TokVersion, LANG_SPEC_STRUCTURAL_ROLE, "constant.numeric.version", 1, allocPattern(buildVersionSemverV3Pattern(f, t))),
		tokenDef(TokKWLSpec, LANG_SPEC_STRUCTURAL_ROLE, "keyword.declaration.lspec", 1, allocPattern(pattern.LiteralString(f, "lspec"))),
	}

	// Headers: same structure as former DSLHeaderSpec
	spec.Headers = []DSLHeaderNestStep{
		defHeaderStep("expect_name", "meta.block.header",
			defHeaderExpect(GrammarIDDSLName, NodeDSLName, []LangSpecLexerTokenType{TokStringLiteral}, false, DSLNestActionPushNext, 0, "entity.name.language", false),
		),
		defHeaderStep("expect_version", "",
			defHeaderExpect(GrammarIDDSLVersion, NodeVersion, []LangSpecLexerTokenType{TokVersion}, false, DSLNestActionPushNext, 0, "", false),
		),
		defHeaderStep("expect_tail", "",
			defHeaderExpect(GrammarIDHeaderSeparator, 0, []LangSpecLexerTokenType{TokDashes}, true, DSLNestActionPop, 3, "", true),
			defHeaderExpect(GrammarIDHeaderSeparator, 0, []LangSpecLexerTokenType{TokHeaderSeparator}, true, DSLNestActionMatch, 0, "", false),
			defHeaderExpect(GrammarIDLangspecName, NodeLSPECName, []LangSpecLexerTokenType{TokStringLiteral, TokKWLSpec}, false, DSLNestActionMatch, 0, "", false),
			defHeaderExpect(GrammarIDLangspecVersion, NodeVersion, []LangSpecLexerTokenType{TokVersion}, false, DSLNestActionMatch, 0, "", false),
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
