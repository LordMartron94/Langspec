package dsl

import (
	"autarch/pattern"
	"foundation/domain"
	"syntaxa"
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

	// -- Punctuation & Operators --
	TokDashes
	TokHeaderSeparator
	TokBraceOpen
	TokBraceClose
	TokSemicolon
	TokComma
	TokChainSeparator
	TokRuleAssignment
	TokEqualsOperator
	TokMetaSection
	TokIdentifier

	// -- Literals --
	TokStringLiteral
	TokRegexLiteral
	TokVersion
	TokInteger

	// -- Keywords --
	TokKWLSpec
	TokKWLex
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
	NodeIdentifier

	// -- Lex Elements --
	NodeLexKeyword
	NodeLexRule
	NodeLexRuleTokenName
	NodeLexRuleScope
	NodeLexRulePattern
	NodeLexRulePriority
)

const (
	// Root & General
	GrammarIDProgram    syntaxa.GrammarID = "PROGRAM"
	GrammarIDEOF        syntaxa.GrammarID = "EOF"
	GrammarIDBlockClose syntaxa.GrammarID = "BLOCK CLOSE"

	// Header
	GrammarIDHeader          syntaxa.GrammarID = "HEADER"
	GrammarIDHeaderContent   syntaxa.GrammarID = "HEADER CONTENT"
	GrammarIDHeaderSeparator syntaxa.GrammarID = "HEADER SEPARATOR"
	GrammarIDHeaderDashes    syntaxa.GrammarID = "HEADER DASHES"
	GrammarIDDSLName         syntaxa.GrammarID = "DSL NAME"
	GrammarIDDSLVersion      syntaxa.GrammarID = "DSL VERSION"
	GrammarIDLangspecName    syntaxa.GrammarID = "LANGSPEC NAME"
	GrammarIDLangspecVersion syntaxa.GrammarID = "LANGSPEC VERSION"

	// Pragma Section
	GrammarIDPragmaSection       syntaxa.GrammarID = "PRAGMA SECTION"
	GrammarIDPragmaStatement     syntaxa.GrammarID = "PRAGMA STATEMENT"
	GrammarIDPragmaStatementBody syntaxa.GrammarID = "PRAGMA STATEMENT BODY"
	GrammarIDPragmaStart         syntaxa.GrammarID = "PRAGMA START"
	GrammarIDPragmaEnd           syntaxa.GrammarID = "PRAGMA END"

	GrammarIDPragmaKey   syntaxa.GrammarID = "PRAGMA KEY"
	GrammarIDPragmaValue syntaxa.GrammarID = "PRAGMA VALUE"

	// Meta Section
	GrammarIDMetaSection      syntaxa.GrammarID = "META SECTION"
	GrammarIDMetaKeyValuePair syntaxa.GrammarID = "META KEY VALUE PAIR"
	GrammarIDMetaKeyValueSeq  syntaxa.GrammarID = "META KEY VALUE SEQUENCE"

	GrammarIDMetaKey        syntaxa.GrammarID = "META KEY"
	GrammarIDMetaValue      syntaxa.GrammarID = "META VALUE"
	GrammarIDMetaAssignment syntaxa.GrammarID = "META ASSIGNMENT"

	// Lex Section
	GrammarIDLexSection       syntaxa.GrammarID = "LEX SECTION"
	GrammarIDLexSectionBody   syntaxa.GrammarID = "LEX SECTION BODY"
	GrammarIDLexRuleList      syntaxa.GrammarID = "LEX RULE LIST"
	GrammarIDLexKeyword       syntaxa.GrammarID = "LEX KEYWORD"
	GrammarIDLexRule          syntaxa.GrammarID = "LEX RULE"
	GrammarIDLexRuleTokenName syntaxa.GrammarID = "LEX RULE TOKEN NAME"
	GrammarIDLexRuleScope     syntaxa.GrammarID = "LEX RULE SCOPE"
	GrammarIDLexRulePattern   syntaxa.GrammarID = "LEX RULE PATTERN"
	GrammarIDLexRulePriority  syntaxa.GrammarID = "LEX RULE PRIORITY"
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

		DefineToken(TokRuleAssignment).
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

		DefineToken(TokIdentifier).
			Scope("variable.other").
			HighPriority().
			Pattern(buildIdentifierPattern(f)).
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
parsing engine's RuleBuilder. All grammar construction for the DSL lives here.
*/
func buildProgramRule(ruleBuilder *RuleBuilder, spec LanguageSpec) Rule {
	headerRule := buildHeaderRule(ruleBuilder, spec)
	bodyRule := buildLexSectionRule(ruleBuilder)
	pragmaRule := buildPragmaRule(ruleBuilder)
	return ruleBuilder.Rule.Root(
		GrammarIDProgram,
		NodeProgram,
		false,
		ruleBuilder.Rule.Required(headerRule, "must have header"),
		ruleBuilder.Rule.TransparentZeroOrMore(GrammarIDPragmaSection, pragmaRule),
		ruleBuilder.Rule.Required(bodyRule, "must have lex ruleset"),
		ruleBuilder.Token.ExpectVirtual(GrammarIDEOF, TokEOF),
	)
}

func buildHeaderRule(ruleBuilder *RuleBuilder, spec LanguageSpec) Rule {
	flat := LanguageSpecHeaderExpectationsFlat(spec)
	sequenceRules := make([]Rule, 0, len(flat))
	for _, e := range flat {
		if e.Virtual {
			if len(e.Tokens) > 0 {
				sequenceRules = append(sequenceRules, ruleBuilder.Token.ExpectVirtual(e.GrammarID, e.Tokens[0]))
			}
			continue
		}
		if len(e.Tokens) == 1 {
			sequenceRules = append(sequenceRules, ruleBuilder.Token.Expect(e.GrammarID, e.NodeKind, e.Tokens[0]))
		} else if len(e.Tokens) > 1 {
			sequenceRules = append(sequenceRules, ruleBuilder.Token.ExpectOneOf(e.GrammarID, e.NodeKind, e.Tokens...))
		}
	}

	headerContent := ruleBuilder.Rule.Sequence(
		GrammarIDHeaderContent,
		NodeHeader,
		sequenceRules...,
	)

	return ruleBuilder.Rule.TransparentNest(
		GrammarIDHeader,
		TokDashes, TokDashes,
		headerContent,
	)
}

func buildPragmaRule(ruleBuilder *RuleBuilder) Rule {
	pragmaBody := ruleBuilder.Rule.Sequence(
		GrammarIDPragmaStatementBody,
		NodePragmaStatement,
		ruleBuilder.Token.Expect(GrammarIDPragmaKey, NodePragmaKey, TokIdentifier),
		ruleBuilder.Token.Expect(GrammarIDPragmaValue, NodePragmaValue, TokStringLiteral),
	)

	return ruleBuilder.Rule.Nest(
		GrammarIDPragmaStatement,
		NodePragmaStatement,
		TokPragmaStart, TokSemicolon,
		pragmaBody,
	)
}

func buildLexSectionRule(ruleBuilder *RuleBuilder) Rule {
	lexSection := ruleBuilder.Rule.Sequence(
		GrammarIDLexSection,
		NodeLexSection,
		ruleBuilder.Token.Expect(GrammarIDLexKeyword, NodeLexKeyword, TokKWLex),
		ruleBuilder.Rule.TransparentNest(
			GrammarIDLexSectionBody,
			TokBraceOpen, TokBraceClose,
			buildLexRuleList(ruleBuilder),
		),
		ruleBuilder.Token.ExpectVirtual(GrammarIDLexSection, TokSemicolon),
	)

	return lexSection
}

func buildLexRuleList(ruleBuilder *RuleBuilder) Rule {
	lexRule := buildLexRule(ruleBuilder)
	lexRuleWithRecovery := ruleBuilder.Rule.RecoverSync(lexRule, TokSemicolon)

	return ruleBuilder.Rule.TransparentZeroOrMore(
		GrammarIDLexRuleList,
		lexRuleWithRecovery,
	)
}

func buildLexRule(ruleBuilder *RuleBuilder) Rule {
	metaSectionRule := buildMetaSectionRule(ruleBuilder)

	return ruleBuilder.Rule.Sequence(
		GrammarIDLexRule,
		NodeLexRule,
		ruleBuilder.Rule.Optional(
			ruleBuilder.Token.Expect(GrammarIDLexRulePriority, NodeLexRulePriority, TokInteger),
		),
		ruleBuilder.Token.Expect(GrammarIDLexRuleTokenName, NodeLexRuleTokenName, TokStringLiteral),
		ruleBuilder.Token.ExpectVirtual(GrammarIDLexRule, TokChainSeparator),
		ruleBuilder.Token.Expect(GrammarIDLexRuleScope, NodeLexRuleScope, TokStringLiteral),
		ruleBuilder.Token.ExpectVirtual(GrammarIDLexRule, TokRuleAssignment),
		ruleBuilder.Token.Expect(GrammarIDLexRulePattern, NodeLexRulePattern, TokRegexLiteral),
		ruleBuilder.Rule.Optional(metaSectionRule),
		ruleBuilder.Token.ExpectVirtual(GrammarIDLexRule, TokSemicolon),
	)
}

func buildMetaSectionRule(ruleBuilder *RuleBuilder) Rule {
	metaBodyRule := buildMetaSectionBodyRule(ruleBuilder)

	return ruleBuilder.Rule.Nest(
		GrammarIDMetaSection,
		NodeMetaSection,
		TokMetaSection, TokMetaSection, // Open, Close
		metaBodyRule,
	)
}

func buildMetaSectionBodyRule(ruleBuilder *RuleBuilder) Rule {
	return ruleBuilder.Rule.TransparentZeroOrMore(
		GrammarIDMetaKeyValuePair,
		ruleBuilder.Rule.Sequence(
			GrammarIDMetaKeyValueSeq,
			NodeMetaKeyValuePair,
			ruleBuilder.Token.Expect(GrammarIDMetaKey, NodeMetaKey, TokIdentifier),
			ruleBuilder.Token.ExpectVirtual(GrammarIDMetaAssignment, TokEqualsOperator),
			ruleBuilder.Token.Expect(GrammarIDMetaValue, NodeMetaValue, TokStringLiteral),
		),
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
