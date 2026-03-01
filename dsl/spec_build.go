package dsl

import (
	"autarch/pattern"
	"foundation/domain"
	"syntaxa"
	"syntaxa/rule"
)

// ----------------------------------------------------------- LEXER / PARSER ENUMS (single place for definition)

//go:generate stringer -type LangSpecLexerState
type LangSpecLexerState uint8

const (
	LANG_SPEC_LEXER_STATE_DEFAULT LangSpecLexerState = iota + 1
)

//go:generate stringer -type LangSpecLexerTokenType
type LangSpecLexerTokenType uint32

const (
	// -- Control & Whitespace --
	TokEOF LangSpecLexerTokenType = iota + 1
	TokWhitespace
	TokLineComment
	TokBlockComment
	TokPragmaStart
	TokVarRef // $

	// -- Punctuation & Operators --
	TokDashes
	TokBraceOpen
	TokBraceClose
	TokSemicolon
	TokComma
	TokChainSeparator
	TokAssignment // :
	TokEqualsOperator
	TokMetaSection // %%

	TokPipe
	TokConcat
	TokRange
	TokStar

	// -- Literals --
	TokStringLiteral
	TokRegexLiteral
	TokCharLiteral
	TokVersion
	TokInteger

	TokIdentifier

	// -- Keywords --
	TokKWLSpec
	TokKWLex
	TokKWPattern
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

	// -- Root & Top Level --
	NodeProgram
	NodeHeader
	NodeLexSection

	// Pragmas
	NodePragmaStatement
	NodePragmaKey
	NodePragmaValue

	// Meta
	NodeMetaSection
	NodeMetaKeyValuePair

	NodeMetaKey
	NodeMetaValue

	// -- Header Elements --
	NodeDSLName
	NodeVersion
	NodeLSPECName

	// -- Lex Elements --
	NodeLexKeyword
	NodeLexRule
	NodeLexRuleTokenName
	NodeLexRuleScope
	NodeLexRulePattern
	NodeLexRulePriority

	// -- Pattern Section --
	NodePatternSection
	NodePatternKeyword
	NodePatternDefinition
	NodePatternDefName
	NodePatternVarRef
	NodePatternRange
	NodePatternCharLiteral
	NodePatternStar
	NodePatternConcat
	NodePatternAlternation
)

// ----------------------------------------------------------- SINGLE SOURCE OF TRUTH

/*
buildLanguageSpec is the only place where language constructs are declared.
Returns a LanguageSpec with tokens (each carrying its pattern directly) and header steps.
No init() or package-level registries; all definition data lives in the returned value.
*/
func buildLanguageSpec(f *pattern.RegulaASTFactory[rune], t *pattern.RegulaTemplates[rune]) LanguageSpec {
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

		DefineToken(TokInteger).
			Scope("constant.numeric").
			Pattern(t.Digit()).
			Build(),

		DefineToken(TokMetaSection).
			Scope("punctuation.section.meta").
			Pattern(pattern.LiteralString(f, "%%")).
			Build(),

		DefineToken(TokPipe).
			Scope("punctuation.pipe").
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

		DefineToken(TokAssignment).
			Scope("keyword.operator.assignment").
			Pattern(f.Literal(':')).
			Build(),

		DefineToken(TokChainSeparator).
			Scope("punctuation.separator.chain").
			Pattern(pattern.LiteralString(f, "->")).
			Build(),

		DefineToken(TokEqualsOperator).
			Scope("keyword.operator.assignment").
			Pattern(pattern.LiteralString(f, "=")).
			Build(),

		DefineToken(TokPragmaStart).
			Scope("punctuation.definition.pragma").
			Pattern(pattern.LiteralString(f, "#")).
			Build(),

		DefineToken(TokStringLiteral).
			Scope("string.quoted.double").
			HighPriority().
			Pattern(buildStringLiteralPattern(f)).
			Build(),

		DefineToken(TokRegexLiteral).
			Scope("string.regexp").
			HighPriority().
			Pattern(buildRawStringLiteral(f)).
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

		DefineToken(TokKWLex).
			Scope("keyword.declaration.lex").
			HighPriority().
			Pattern(pattern.LiteralString(f, "LEX")).
			Build(),

		DefineToken(TokKWPattern).
			Scope("keyword.declaration.pattern").
			HighPriority().
			Pattern(pattern.LiteralString(f, "PATTERN")).
			Build(),

		DefineToken(TokVarRef).
			Scope("keyword.operator.variable").
			Pattern(f.Literal('$')).
			Build(),

		DefineToken(TokConcat).
			Scope("keyword.operator.concat").
			Pattern(f.Literal('&')).
			Build(),

		DefineToken(TokRange).
			Scope("keyword.operator.range").
			Pattern(pattern.LiteralString(f, "..")).
			Build(),

		DefineToken(TokStar).
			Scope("keyword.operator.star").
			Pattern(f.Literal('*')).
			Build(),

		DefineToken(TokCharLiteral).
			Scope("string.quoted.single").
			HighPriority().
			Pattern(buildCharLiteralPattern(f)).
			Build(),

		DefineToken(TokIdentifier).
			Scope("variable.other").
			HighPriority().
			Pattern(buildIdentifierPattern(f)).
			Build(),
	}

	spec.Headers = []DSLHeaderNestStep{
		defHeaderStep("expect_name", "meta.block.header",
			ExpectNode(NodeDSLName).
				Tokens(TokStringLiteral).
				PushNext().
				ScopeOverride("entity.name.language").
				Build(),
		),
		defHeaderStep("expect_version", "",
			ExpectNode(NodeVersion).
				GrammarID("DSL VERSION").
				Tokens(TokVersion).
				PushNext().
				Build(),
		),
		defHeaderStep("expect_tail", "",
			ExpectVirtual(VirtualHeaderDashes).
				Tokens(TokDashes).
				Pop(3).
				NestOnly().
				Build(),

			ExpectVirtual(VirtualHeaderSeparator).
				Tokens(TokPipe).
				Build(),

			ExpectNode(NodeLSPECName).
				Tokens(TokStringLiteral, TokKWLSpec).
				Build(),

			ExpectNode(NodeVersion).
				GrammarID("LANGSPEC VERSION").
				Tokens(TokVersion).
				Build(),
		),
	}

	return spec
}

/*
dslSpecFactoryAndTemplates creates the factory and templates used by BuildLanguageSpec.
Exported so the compiler or spec_builder can create them without depending on package-level state.
*/
func dslSpecFactoryAndTemplates() (*pattern.RegulaASTFactory[rune], *pattern.RegulaTemplates[rune]) {
	f := pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())
	t := pattern.RegulaTemplatesCreate(f)
	return f, t
}

/*
buildProgramRule builds the full program rule (header + optional LEX body + EOF) using the
GrammarDefiner. All grammar construction for the DSL lives here.
*/
func buildProgramRule(g *GrammarDefiner, spec LanguageSpec) Rule {
	headerRule := buildHeaderRule(g, spec)
	pragmaRule := buildPragmaRule(g)

	lexSectionRule := buildLexSectionRule(g)

	return g.rb.Rule.Root(
		LangSpecGrammarIDFromNode(NodeProgram, ""),
		NodeProgram,
		false,
		g.rb.Rule.Required(headerRule, "must have header"),
		g.rb.Rule.TransparentZeroOrMore(LangSpecGrammarIDFromNode(NodePragmaStatement, ""), pragmaRule),
		g.rb.Rule.Optional(buildPatternSectionRule(g)),
		g.rb.Rule.Required(lexSectionRule, "must have lex ruleset"),
		g.expectVirtual(VirtualEOF, TokEOF),
	)
}

func buildHeaderRule(g *GrammarDefiner, spec LanguageSpec) Rule {
	flat := LanguageSpecHeaderExpectationsFlat(spec)
	sequenceRules := langSpecBuildHeaderRules(g, flat)
	headerContent := g.rb.Rule.Sequence(
		LangSpecGrammarIDFromNode(NodeHeader, "CONTENT"),
		NodeHeader,
		sequenceRules...,
	)
	return g.rb.Rule.TransparentNest(
		LangSpecGrammarIDFromNode(NodeHeader, ""),
		TokDashes, TokDashes,
		headerContent,
	)
}

func buildPragmaRule(g *GrammarDefiner) Rule {
	pragmaBody := g.sequence(NodePragmaStatement, "").
		expectToken(NodePragmaKey, TokIdentifier).
		expectToken(NodePragmaValue, TokStringLiteral).
		build()
	return g.rb.Rule.TransparentNest(
		LangSpecGrammarIDFromNode(NodePragmaStatement, ""),
		TokPragmaStart, TokSemicolon,
		pragmaBody,
	)
}

func buildLexSectionRule(g *GrammarDefiner) Rule {
	return g.block(
		NodeLexSection,
		NodeLexKeyword,
		TokKWLex,
		buildLexRuleList(g),
	)
}

func buildLexRuleList(g *GrammarDefiner) Rule {
	lexRule := buildLexRule(g)
	lexRuleWithRecovery := g.rb.Rule.RecoverSync(lexRule, TokSemicolon)
	return g.rb.Rule.TransparentZeroOrMore(
		LangSpecGrammarIDFromNode(NodeLexRule, "LIST"),
		lexRuleWithRecovery,
	)
}

func buildLexRule(g *GrammarDefiner) Rule {
	return g.sequence(NodeLexRule, "").
		optionalToken(NodeLexRulePriority, TokInteger).
		expectToken(NodeLexRuleTokenName, TokStringLiteral).
		expectVirtualInRule(TokChainSeparator).
		expectToken(NodeLexRuleScope, TokStringLiteral).
		expectVirtualInRule(TokAssignment).
		expectToken(NodeLexRulePattern, TokRegexLiteral).
		optionalRule(buildMetaSectionRule(g)).
		expectVirtualInRule(TokSemicolon).
		build()
}

// ----------------------------------------------------------- PATTERN SECTION

func buildPatternSectionRule(g *GrammarDefiner) Rule {
	return g.block(
		NodePatternSection,
		NodePatternKeyword,
		TokKWPattern,
		buildPatternDefinitionList(g),
	)
}

func buildPatternDefinitionList(g *GrammarDefiner) Rule {
	defRule := buildPatternDefinition(g)
	defWithRecovery := g.rb.Rule.RecoverSync(defRule, TokSemicolon, TokBraceClose)
	return g.rb.Rule.TransparentZeroOrMore(
		LangSpecGrammarIDFromNode(NodePatternDefinition, "LIST"),
		defWithRecovery,
	)
}

func buildPatternDefinition(g *GrammarDefiner) Rule {
	return g.sequence(NodePatternDefinition, "").
		expectToken(NodePatternDefName, TokIdentifier).
		expectVirtualInRule(TokAssignment).
		requiredRule(buildPatternExprRule(g), "pattern definition must have an expression").
		expectVirtualInRule(TokSemicolon).
		build()
}

func buildPatternExprRule(g *GrammarDefiner) Rule {
	segmentPrimary := g.rb.Rule.OptionalSuffix(
		LangSpecGrammarIDFromNode(NodePatternStar, ""),
		NodePatternStar,
		buildPatternSegmentRule(g),
		TokStar,
	)
	cfg := rule.PrattConfig[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
		Primary:        segmentPrimary,
		PrefixOps:      nil,
		InfixOps:       nil,
		RecoveryTokens: []LangSpecLexerTokenType{TokSemicolon, TokBraceClose},
	}
	cfg.InfixOps = []rule.PrattInfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
		{Token: TokConcat, LeftBP: 20, RightBP: 19, NodeKind: NodePatternConcat},
		{Token: TokPipe, LeftBP: 10, RightBP: 9, NodeKind: NodePatternAlternation},
	}
	return g.rb.Pratt.Expression(LangSpecGrammarIDFromNode(NodePatternConcat, ""), cfg)
}

func buildPatternSegmentRule(g *GrammarDefiner) Rule {
	rangeRule := g.rb.Rule.Predict(
		buildPatternRangeRule(g),
		func(ctx *syntaxa.SelectRuleContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole]) bool {
			return ctx.Peek(1).Token == TokRange
		},
	)
	return g.rb.Rule.Choice(
		LangSpecGrammarIDFromNode(NodePatternVarRef, ""),
		buildPatternVarRefRule(g),
		rangeRule,
		buildPatternCharLiteralRule(g),
	)
}

func buildPatternVarRefRule(g *GrammarDefiner) Rule {
	return g.expectPair(NodePatternVarRef, TokVarRef, TokIdentifier)
}

func buildPatternRangeRule(g *GrammarDefiner) Rule {
	return g.sequence(NodePatternRange, "").
		expectToken(NodePatternCharLiteral, TokCharLiteral).
		expectVirtualInRule(TokRange).
		expectToken(NodePatternCharLiteral, TokCharLiteral).
		build()
}

func buildPatternCharLiteralRule(g *GrammarDefiner) Rule {
	return g.expectToken(NodePatternCharLiteral, TokCharLiteral)
}

func buildMetaSectionRule(g *GrammarDefiner) Rule {
	metaBodyRule := buildMetaSectionBodyRule(g)
	return g.rb.Rule.Nest(
		LangSpecGrammarIDFromNode(NodeMetaSection, ""),
		NodeMetaSection,
		TokMetaSection, TokMetaSection,
		metaBodyRule,
	)
}

func buildMetaSectionBodyRule(g *GrammarDefiner) Rule {
	return g.rb.Rule.TransparentZeroOrMore(
		LangSpecGrammarIDFromNode(NodeMetaKeyValuePair, ""),
		g.sequence(NodeMetaKeyValuePair, "SEQUENCE").
			expectToken(NodeMetaKey, TokIdentifier).
			expectVirtual(VirtualMetaAssignment, TokEqualsOperator).
			expectToken(NodeMetaValue, TokStringLiteral).
			build(),
	)
}

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

func buildCharLiteralPattern(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	normalChar := f.NegatedClass(
		f.Range('\'', '\''),
		f.Range('\\', '\\'),
		f.Range('\n', '\n'),
		f.Range('\r', '\r'),
	)

	escapeTrigger := f.Literal('\\')

	escapedChar := f.Class(
		f.Range('\'', '\''),
		f.Range('\\', '\\'),
		f.Range('n', 'n'),
		f.Range('r', 'r'),
		f.Range('t', 't'),
	)

	escapeSequence := f.Sequence(escapeTrigger, escapedChar)

	charContent := f.AnyOf(normalChar, escapeSequence)

	quote := f.Literal('\'')

	return f.Sequence(quote, charContent, quote)
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

func buildRawStringLiteral(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	quote := f.Literal('`')
	body := buildRawStringBody(f)

	return f.Sequence(quote, body, quote)
}

func buildRawStringBody(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	normalChar := f.NegatedClass(
		f.Range('`', '`'),
		f.Range('\\', '\\'),
	)

	escapeSequence := buildPassThroughEscape(f)

	return f.AnyOf(normalChar, escapeSequence).Star()
}

func buildPassThroughEscape(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	escapeTrigger := f.Literal('\\')
	anyChar := f.NegatedClass()

	return f.Sequence(escapeTrigger, anyChar)
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

func buildIdentifierPattern(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	start := buildIdentifierStartChar(f)
	body := buildIdentifierBodyChars(f)

	return f.Sequence(start, body)
}

func buildIdentifierStartChar(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	return f.Class(
		f.Range('a', 'z'),
		f.Range('A', 'Z'),
		f.Range('_', '_'),
	)
}

func buildIdentifierBodyChars(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	validChar := f.Class(
		f.Range('a', 'z'),
		f.Range('A', 'Z'),
		f.Range('0', '9'),
		f.Range('_', '_'),
		f.Range('-', '-'),
	)

	return validChar.Star()
}
