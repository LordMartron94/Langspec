package spec

import (
	"autarch/pattern"
	"foundation/domain"
	"syntaxa"
	"syntaxa/rule"
)

// ----------------------------------------------------------- LEXER / PARSER ENUMS

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

	// -- Punctuation & Operators --
	TokDashes
	TokBraceOpen
	TokBraceClose
	TokParenOpen
	TokParenClose
	TokBracketOpen
	TokBracketClose
	TokSemicolon
	TokDot
	TokChainSeparator
	TokAssignment // :
	TokEqualsOperator
	TokMetaSection // %%
	TokComma

	TokPipe
	TokRange
	TokStar
	TokSeparator // .
	TokNegation  // !
	TokPlus      // +
	TokOptional  // ?

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
	TokKWPragma
	TokKWTool
	TokKWParse

	TokKWTrue
	TokKWFalse
	TokKWLocal
	TokKWVirtual
	TokKWNest
	TokKWPratt

	TokKWPrimary
	TokKWPrefix
	TokKWPostfix
	TokKWInfix
	TokKWImplicit
	TokKWPrecedence
	TokKWTransparent
	TokKWSync
	TokKWPredict

	TokKWIgnore
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
	NodePragmaKeyword
	NodePragmaSection

	NodePragmaBlock
	NodePragmaBlockKey
	NodePragmaBlockKeyPrefix
	NodePragmaBlockKeySegment

	NodePragmaConfiguration
	NodePragmaKey
	NodePragmaValue

	NodeStringArray

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
	NodeLexRuleRole
	NodeLexRulePattern
	NodeLexRulePriority

	// -- Pattern Section --
	NodePatternSection
	NodePatternKeyword
	NodePatternDefinition
	NodePatternAny
	NodePatternRegEx
	NodePatternDefName
	NodePatternRef
	NodePatternRange
	NodeCharLiteral
	NodePatternStar
	NodePatternPlus
	NodePatternNegation
	NodePatternConcat
	NodePatternAlternation
	NodePatternGroup
	NodePatternSegment
	NodePatternOptional
	NodeRepetition
	NodeRepetitionBounds
	NodeRepetitionMin
	NodeRepetitionMax
	NodePatternClass
	NodePatternClassItem

	NodeLocalVariable

	// -- Parse Section --
	NodeParseSection
	NodeParseKeyword
	NodeParseRule

	NodeParseRuleName
	NodeParseNodeName
	NodeParseRuleBody
	NodeParseOpSuppress
	NodeParseAlternation
	NodeParseConcat
	NodeParseOptional
	NodeParseSegment
	NodeParseGroup

	NodeParseSymbolReference
	NodeParseTokenReference
	NodeParseExpressionReference

	NodeParseOpNest
	NodeParseNestOpenToken
	NodeParseNestCloseToken
	NodeParseNestBody

	NodeParseStar
	NodeParsePlus

	NodeParseSectionBody
	NodeParseIgnoreSection
	NodeParseIgnoreKeyword
	NodeParseIgnoreRole

	NodeRuleModifierTransparent
	NodeRuleModifierSync
	NodeSyncBlock
	NodeSyncToken

	NodeParseModifierPredict
	NodePredictLookaheadList
	NodePredictLookahead
	NodePredictOffset
	NodePredictToken

	// -- Pratt Section --

	NodePrattSection
	NodePrattKeyword
	NodePrattExprDef
	NodePrattExprName
	NodePrattExprBody
	NodePrattCategoryList
	NodePrattCategory
	NodePrattPrimary
	NodePrattPrimaryBody
	NodePrattPrimaryRef

	NodePrattImplicit
	NodePrattImplicitBody
	NodePrattImplicitDef

	NodePrattOperatorTarget
	NodeStringLiteral

	NodePrattPrefix
	NodePrattPrefixBody
	NodePrattPrefixList

	NodePrattPostfix
	NodePrattPostfixBody
	NodePrattPostfixList

	NodePrattInfix
	NodePrattInfixBody
	NodePrattInfixList

	NodePrattOperatorDef

	NodePrattLeftPrecedenceValue
	NodePrattRightPrecedenceValue
)

// ----------------------------------------------------------- LEXER DEFINITION

type staticToken struct {
	Token    LangSpecLexerTokenType
	Scope    string
	Match    string
	Priority int
}

/*
buildLanguageSpec is the only place where language constructs are declared.
Returns a LanguageSpec with tokens and header steps.
*/
func buildLanguageSpec(f *pattern.RegulaASTFactory[rune], t *pattern.RegulaTemplates[rune]) LanguageSpec {
	spec := LanguageSpec{}

	// 1. Table-Driven Static Tokens
	statics := []staticToken{
		{TokDashes, "punctuation.definition.separator", "---", 0},
		{TokMetaSection, "punctuation.section.meta", "%%", 0},
		{TokPipe, "punctuation.pipe", "|", 0},
		{TokDot, "punctuation.separator.dot", ".", 0},
		{TokSemicolon, "punctuation.terminator.statement", ";", 0},
		{TokBraceOpen, "punctuation.section.braces.begin", "{", 0},
		{TokBraceClose, "punctuation.section.braces.end", "}", 0},
		{TokParenOpen, "punctuation.section.parens.begin", "(", 0},
		{TokParenClose, "punctuation.section.parens.end", ")", 0},
		{TokBracketOpen, "punctuation.section.brackets.begin", "[", 0},
		{TokBracketClose, "punctuation.section.brackets.end", "]", 0},
		{TokComma, "punctuation.separator.comma", ",", 0},
		{TokPlus, "keyword.operator.plus", "+", 0},
		{TokNegation, "keyword.operator.negation", "!", 0},
		{TokAssignment, "keyword.operator.assignment", ":", 0},
		{TokChainSeparator, "punctuation.separator.chain", "->", 0},
		{TokEqualsOperator, "keyword.operator.assignment", "=", 0},
		{TokOptional, "keyword.operator.optional", "?", 0},
		{TokRange, "keyword.operator.range", "..", 0},
		{TokStar, "keyword.operator.star", "*", 0},
		{TokKWLSpec, "keyword.lspec", "lspec", 2},
		{TokKWPragma, "keyword.pragma", "PRAGMA", 2},
		{TokKWTool, "keyword.tool", "tool", 2},
		{TokKWLex, "keyword.declaration.lex", "LEX", 2},
		{TokKWPattern, "keyword.declaration.pattern", "PATTERN", 2},
		{TokKWPratt, "keyword.declaration.pratt", "PRATT", 2},
		{TokKWTrue, "constant.language.boolean", "true", 2},
		{TokKWFalse, "constant.language.boolean", "false", 2},
		{TokKWLocal, "keyword.modifier.local", "local", 2},
		{TokKWParse, "keyword.declaration.parse", "PARSE", 2},
		{TokKWVirtual, "keyword.operator.virtual", "virtual", 2},
		{TokKWNest, "keyword.operator.nest", "nest", 2},
		{TokKWPrimary, "keyword.pratt.primary", "primary", 2},
		{TokKWPrefix, "keyword.pratt.prefix", "prefix", 2},
		{TokKWPostfix, "keyword.pratt.postfix", "postfix", 2},
		{TokKWInfix, "keyword.pratt.infix", "infix", 2},
		{TokKWImplicit, "keyword.pratt.implicit", "implicit", 2},
		{TokKWPrecedence, "keyword.pratt.precedence", "precedence", 2},
		{TokKWIgnore, "keyword.declaration.ignore", "IGNORE", 2},
		{TokKWTransparent, "keyword.operator.transparent", "transparent", 2},
		{TokKWSync, "keyword.operator.sync", "sync", 2},
		{TokKWPredict, "keyword.operator.predict", "predict", 2},
	}

	for _, st := range statics {
		spec.Tokens = append(spec.Tokens, DefineToken(st.Token).
			Scope(st.Scope).
			Priority(st.Priority).
			Pattern(pattern.LiteralString(f, st.Match)).
			Build())
	}

	// 2. Complex & Dynamic Tokens
	spec.Tokens = append(spec.Tokens,
		DefineToken(TokEOF).
			Scope("meta.eof").
			Pattern(f.Class()).
			Build(),

		DefineToken(TokWhitespace).
			Role(LANG_SPEC_WHITESPACE_ROLE).
			Scope("punctuation.whitespace").
			Pattern(t.Whitespace().Plus()).
			Build(),

		DefineToken(TokLineComment).
			Role(LANG_SPEC_COMMENT_ROLE).
			Scope("comment.line.double-slash").
			PatternFromRegex(`//[^\n\r]*`, f).
			Build(),

		DefineToken(TokBlockComment).
			Role(LANG_SPEC_COMMENT_ROLE).
			Scope("comment.block").
			Delimited(buildBlockCommentOpenPattern(f), buildBlockCommentClosePattern(f)).
			Pattern(buildBlockCommentPattern(f)).
			Build(),

		DefineToken(TokInteger).
			Scope("constant.numeric").
			PatternFromRegex("[0-9]+", f).
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
			PatternFromRegex(`v[0-9]+\.[0-9]+\.[0-9]+`, f).
			Build(),

		DefineToken(TokCharLiteral).
			Scope("string.quoted.single").
			HighPriority().
			Pattern(buildCharLiteralPattern(f)).
			Build(),

		DefineToken(TokIdentifier).
			Scope("variable.other").
			HighPriority().
			PatternFromRegex(`[a-zA-Z_][a-zA-Z0-9_\-]*`, f).
			Build(),
	)

	return spec
}

/*
dslSpecFactoryAndTemplates creates the factory and templates used by BuildLanguageSpec.
*/
func dslSpecFactoryAndTemplates() (*pattern.RegulaASTFactory[rune], *pattern.RegulaTemplates[rune]) {
	f := pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())
	t := pattern.RegulaTemplatesCreate(f)
	return f, t
}

// ----------------------------------------------------------- PARSER DEFINITION

type dslGrammarBuilder struct {
	g *GrammarDefiner
}

/*
buildProgramRule builds the full program rule (header + optional LEX body + EOF).
*/
func buildProgramRule(g *GrammarDefiner) Rule {
	builder := &dslGrammarBuilder{g: g}
	return builder.program()
}

func (b *dslGrammarBuilder) program() Rule {
	b.declarePatternRef()

	return b.g.RootByNode(NodeProgram, false,
		b.g.rb.Rule.Required(b.header(), "must have header"),
		b.g.rb.Rule.OptionalPrefix(b.pragmaSection(), TokKWPragma),
		b.g.rb.Rule.OptionalPrefix(b.patternSection(), TokKWPattern),
		b.g.rb.Rule.Required(b.lexSection(), "must have lex ruleset"),
		b.g.rb.Rule.OptionalPrefix(b.prattSection(), TokKWPratt),
		b.g.rb.Rule.Required(b.parseSection(), "must have parse ruleset"),
		b.g.expectVirtualInRule(NodeProgram, TokEOF),
	)
}

// ----------------------------------------------------------- HEADER

func (b *dslGrammarBuilder) header() Rule {
	headerContent := b.g.sequence(NodeHeader, "CONTENT").
		expect(NodeDSLName, "", TokStringLiteral).
		expect(NodeVersion, "DSL", TokVersion).
		expectVirtual(VirtualHeaderSeparator, TokPipe).
		expect(NodeLSPECName, "", TokKWLSpec).
		expect(NodeVersion, "LANGSPEC", TokVersion).
		build()
	return b.g.TransparentNestByNode(NodeHeader, "", TokDashes, TokDashes, headerContent)
}

// ----------------------------------------------------------- PRAGMA SECTION

func (b *dslGrammarBuilder) pragmaSection() Rule {
	blockRule := b.g.block(
		NodePragmaSection,
		NodePragmaKeyword,
		TokKWPragma,
		b.pragmaBlockList(),
	)
	return b.g.rb.Rule.RecoverSync(blockRule, TokSemicolon, TokBraceClose)
}

func (b *dslGrammarBuilder) pragmaBlockList() Rule {
	return b.g.TransparentZeroOrMoreByNode(NodePragmaBlock, "LIST", b.pragmaBlock())
}

func (b *dslGrammarBuilder) pragmaBlock() Rule {
	blockRule := b.g.blockByRule(
		NodePragmaBlock,
		b.blockKey(),
		b.pragmaConfigurationList(),
	)
	return b.g.rb.Rule.RecoverSync(blockRule, TokSemicolon, TokBraceClose)
}

func (b *dslGrammarBuilder) blockKey() Rule {
	return b.g.rb.Scope(LangSpecGrammarIDFromNode(NodePragmaBlockKey)).Path(
		NodePragmaBlockKey,
		NodePragmaBlockKeyPrefix,
		TokKWTool,
		TokDot,
		LangSpecGrammarIDFromNode(NodePragmaBlockKeySegment),
		NodePragmaBlockKeySegment,
		TokIdentifier,
	)
}

func (b *dslGrammarBuilder) pragmaConfigurationList() Rule {
	return b.g.TransparentZeroOrMoreByNode(NodePragmaConfiguration, "LIST", b.pragmaConfiguration())
}

func (b *dslGrammarBuilder) pragmaConfiguration() Rule {
	return b.g.rb.Rule.RecoverSync(b.g.sequence(NodePragmaConfiguration, "").
		expectToken(NodePragmaKey, TokIdentifier).
		expectVirtualInRule(TokEqualsOperator).
		rule(b.g.rb.Rule.Choice(
			"DUMMY CHOICE FOR PRAGMA VALUES",
			b.g.rb.Rule.Wrap(LangSpecGrammarIDFromNode(NodePragmaValue), NodePragmaValue, b.stringArray()),
			b.g.expectOneOf(NodePragmaValue, TokIdentifier, TokStringLiteral, TokKWTrue, TokKWFalse)),
		).
		expectVirtualInRule(TokSemicolon).
		build(), TokSemicolon)
}

func (b *dslGrammarBuilder) stringArray() Rule {
	elementListRule := b.g.rb.Rule.TransparentSequence(
		"STRING_ARRAY_ELEMENT_LIST",
		b.g.rb.Token.Expect(LangSpecGrammarIDFromNode(NodeStringLiteral), NodeStringLiteral, TokStringLiteral),
		b.g.rb.Rule.TransparentZeroOrMore(
			"STRING_ARRAY_ELEMENT_LIST_TAIL",
			b.g.rb.Rule.TransparentSequence(
				"STRING_ARRAY_ELEMENT_LIST_TAIL_CONTENT",
				b.g.rb.Token.ExpectVirtual("COMMA", TokComma),
				b.g.rb.Token.Expect(LangSpecGrammarIDFromNode(NodeStringLiteral), NodeStringLiteral, TokStringLiteral),
			),
		),
	)

	return b.g.rb.Rule.Nest(
		LangSpecGrammarIDFromNode(NodeStringArray), NodeStringArray,
		TokBracketOpen, TokBracketClose,
		elementListRule,
	)
}

// ----------------------------------------------------------- PATTERN SECTION

func (b *dslGrammarBuilder) patternSection() Rule {
	blockRule := b.g.block(
		NodePatternSection,
		NodePatternKeyword,
		TokKWPattern,
		b.patternDefinitionList(),
	)
	return b.g.rb.Rule.RecoverSync(blockRule, TokSemicolon, TokBraceClose)
}

func (b *dslGrammarBuilder) patternDefinitionList() Rule {
	return b.g.TransparentZeroOrMoreByNode(NodePatternDefinition, "LIST", b.patternDefinition())
}

func (b *dslGrammarBuilder) patternDefinition() Rule {
	return b.g.rb.Rule.RecoverSync(b.g.sequence(NodePatternDefinition, "").
		rule(
			b.g.rb.Rule.Choice(
				syntaxa.GrammarLabel("DUMMY CHOICE FOR PATTERNS"),
				b.g.rb.Rule.TransparentSequence(
					syntaxa.GrammarLabel("DUMMY LOCAL VAR FOR PATTERNS"),
					b.g.expectToken(NodeLocalVariable, TokKWLocal),
					b.g.expectToken(NodePatternDefName, TokIdentifier),
				),
				b.g.expectToken(NodePatternDefName, TokIdentifier),
			),
		).
		expectVirtualInRule(TokAssignment).
		requiredRule(b.patternExpr(), "pattern definition must have an expression").
		expectVirtualInRule(TokSemicolon).
		build(), TokSemicolon)
}

func (b *dslGrammarBuilder) patternExpr() Rule {
	cfg := rule.PrattConfig[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
		Primary: b.patternSegment(),

		PrefixOps: []rule.PrattPrefixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			b.g.PrefixOp(TokNegation, 40, NodePatternNegation),
		},

		PostfixOps: []rule.PrattPostfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			b.g.PostfixOp(TokStar, 30, NodePatternStar),
			b.g.PostfixOp(TokPlus, 30, NodePatternPlus),
			b.g.PostfixOp(TokOptional, 30, NodePatternOptional),
		},

		PostfixRuleOps: []rule.PrattPostfixRuleOp[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
			b.g.PostfixRuleOp(30, NodeRepetition, b.patternRepetition()),
		},

		InfixOps: []rule.PrattInfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			b.g.InfixOp(TokPipe, 10, 9, NodePatternAlternation),
			b.g.InfixOp(TokRange, 60, 61, NodePatternRange),
		},

		ImplicitInfix: &rule.PrattImplicitInfix[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			LeftBP:   20,
			RightBP:  19,
			NodeKind: NodePatternConcat,
		},
	}

	return b.g.rb.Pratt.Expression(VirtualGrammarIDToGrammarID(VirtualPatternExpression), cfg)
}

func (b *dslGrammarBuilder) patternSegment() Rule {
	return b.g.ChoiceByNode(NodePatternSegment,
		b.patternRefReference(),
		b.patternClass(),
		b.charLiteral(),
		b.g.expectToken(NodePatternAny, TokDot),
		b.g.expectToken(NodePatternRegEx, TokRegexLiteral),
		b.g.expectToken(NodeStringLiteral, TokStringLiteral),
		b.g.NestByNode(NodePatternGroup, TokParenOpen, TokParenClose,
			b.g.rb.Rule.Reference("PATTERN EXPR REF", VirtualGrammarIDToGrammarID(VirtualPatternExpression))),
	)
}

func (b *dslGrammarBuilder) patternClass() Rule {
	return b.g.NestByNode(
		NodePatternClass,
		TokBracketOpen,
		TokBracketClose,
		b.g.TransparentZeroOrMoreByNode(NodePatternClassItem, "LIST", b.patternClassItem()),
	)
}

func (b *dslGrammarBuilder) patternClassItem() Rule {
	rangeTail := b.g.sequence(NodePatternRange, "TAIL").
		expectVirtualInRule(TokRange).
		expectToken(NodeCharLiteral, TokCharLiteral).
		build()

	item := b.g.sequence(NodePatternClassItem, "SEQ").
		rule(b.charLiteral()).
		optionalRule(rangeTail).
		build()

	return b.g.sequence(NodePatternClassItem, "COMMA").
		rule(item).
		optionalRule(b.g.expectVirtualInRule(NodePatternClassItem, TokComma)).
		build()
}

func (b *dslGrammarBuilder) patternRepetition() Rule {
	return b.g.TransparentNestByNode(
		NodeRepetitionBounds, "NEST",
		TokBraceOpen,
		TokBraceClose,
		b.repetitionBounds(),
	)
}

func (b *dslGrammarBuilder) repetitionBounds() Rule {
	// Single NodeRepetitionBounds for `{min}`, `{min,max}`, `{min,}` — trailing comma + optional max
	// is a transparent fragment so `{min,}` does not allocate a child bounds node with no digits.
	minCommaMaxOpt := b.g.memoize(
		LangSpecGrammarIDFromNodeWithSuffix(NodeRepetitionBounds, "MIN_COMMA_MAX_OPT"),
		func() Rule {
			return b.g.rb.Rule.TransparentSequence(
				LangSpecGrammarIDFromNodeWithSuffix(NodeRepetitionBounds, "MIN_COMMA_MAX_OPT_SEQ"),
				b.g.expectVirtualInRule(NodeRepetitionBounds, TokComma),
				b.g.rb.Rule.Optional(b.g.expectToken(NodeRepetitionMax, TokInteger)),
			)
		},
	)

	startsWithMin := b.g.sequence(NodeRepetitionBounds, "STARTS_WITH_MIN").
		expectToken(NodeRepetitionMin, TokInteger).
		optionalRule(minCommaMaxOpt).
		build()

	startsWithComma := b.g.sequence(NodeRepetitionBounds, "STARTS_WITH_COMMA").
		expectVirtualInRule(TokComma).
		expectToken(NodeRepetitionMax, TokInteger).
		build()

	return b.g.ChoiceByNode(
		NodeRepetitionBounds,
		startsWithMin,
		startsWithComma,
	)
}

// ----------------------------------------------------------- LEX SECTION

func (b *dslGrammarBuilder) lexSection() Rule {
	return b.g.block(
		NodeLexSection,
		NodeLexKeyword,
		TokKWLex,
		b.lexRuleList(),
	)
}

func (b *dslGrammarBuilder) lexRuleList() Rule {
	lexRuleWithRecovery := b.g.rb.Rule.RecoverSync(b.lexRule(), TokSemicolon)
	return b.g.TransparentZeroOrMoreByNode(NodeLexRule, "LIST", lexRuleWithRecovery)
}

func (b *dslGrammarBuilder) lexRule() Rule {
	return b.g.sequence(NodeLexRule, "").
		optionalToken(NodeLexRulePriority, TokInteger).
		expectToken(NodeLexRuleTokenName, TokIdentifier).
		expectVirtualInRule(TokChainSeparator).
		expectToken(NodeLexRuleRole, TokIdentifier).
		expectVirtualInRule(TokAssignment).
		rule(b.g.ChoiceByNode(NodeLexRulePattern,
			b.patternRefReference(),
			b.g.expectToken(NodeLexRulePattern, TokRegexLiteral),
		)).
		optionalRule(b.metaSection()).
		expectVirtualInRule(TokSemicolon).
		build()
}

// ----------------------------------------------------------- LEX: META SECTION

func (b *dslGrammarBuilder) metaSection() Rule {
	return b.g.NestByNode(NodeMetaSection, TokMetaSection, TokMetaSection, b.metaSectionBody())
}

func (b *dslGrammarBuilder) metaSectionBody() Rule {
	return b.g.TransparentZeroOrMoreByNode(NodeMetaKeyValuePair, "",
		b.g.sequence(NodeMetaKeyValuePair, "SEQUENCE").
			expectToken(NodeMetaKey, TokIdentifier).
			expectVirtual(VirtualMetaAssignment, TokEqualsOperator).
			rule(b.g.expectOneOf(NodeMetaValue, TokStringLiteral, TokKWFalse, TokKWTrue)).
			build())
}

// ----------------------------------------------------------- PRATT SECTION

func (b *dslGrammarBuilder) prattSection() Rule {
	blockRule := b.g.block(
		NodePrattSection,
		NodePrattKeyword,
		TokKWPratt,
		b.prattExprDefList(),
	)
	return b.g.rb.Rule.RecoverSync(blockRule, TokSemicolon, TokBraceClose)
}

func (b *dslGrammarBuilder) prattExprDefList() Rule {
	return b.g.TransparentZeroOrMoreByNode(NodePrattExprDef, "LIST", b.prattExprDef())
}

func (b *dslGrammarBuilder) prattExprDef() Rule {
	return b.g.sequence(NodePrattExprDef, "").
		optionalToken(NodeLocalVariable, TokKWLocal).
		expectToken(NodePrattExprName, TokIdentifier).
		optionalRule(b.syncModifier()).
		rule(b.g.NestByNode(
			NodePrattExprBody,
			TokBraceOpen,
			TokBraceClose,
			b.prattCategoryList(),
		)).
		build()
}

func (b *dslGrammarBuilder) prattCategoryList() Rule {
	return b.g.TransparentZeroOrMoreByNode(NodePrattCategoryList, "", b.prattCategory())
}

func (b *dslGrammarBuilder) prattCategory() Rule {
	return b.g.ChoiceByNode(NodePrattCategory,
		b.prattPrimary(),
		b.prattOperatorBlock(NodePrattPrefix, NodePrattPrefixBody, NodePrattPrefixList, TokKWPrefix, b.prattPrefixDef()),
		b.prattOperatorBlock(NodePrattPostfix, NodePrattPostfixBody, NodePrattPostfixList, TokKWPostfix, b.prattPostfixDef()),
		b.prattOperatorBlock(NodePrattInfix, NodePrattInfixBody, NodePrattInfixList, TokKWInfix, b.prattInfixDef()),
		b.prattImplicitBlock(),
	)
}

func (b *dslGrammarBuilder) prattPrimary() Rule {
	return b.g.sequence(NodePrattPrimary, "").
		expectToken(NodePrattKeyword, TokKWPrimary).
		rule(b.g.NestByNode(
			NodePrattPrimaryBody,
			TokBraceOpen,
			TokBraceClose,
			b.prattPrimaryRef(),
		)).
		build()
}

func (b *dslGrammarBuilder) prattPrimaryRef() Rule {
	return b.g.sequence(NodePrattPrimaryRef, "").
		expectToken(NodeParseExpressionReference, TokIdentifier).
		expectVirtualInRule(TokSemicolon).
		build()
}

func (b *dslGrammarBuilder) prattOperatorBlock(
	nodeKind LangSpecParserNodeKind,
	bodyNodeKind LangSpecParserNodeKind,
	listNodeKind LangSpecParserNodeKind,
	keyword LangSpecLexerTokenType,
	defRule Rule,
) Rule {
	return b.g.sequence(nodeKind, "").
		expectToken(NodePrattKeyword, keyword).
		rule(b.g.NestByNode(
			bodyNodeKind,
			TokBraceOpen,
			TokBraceClose,
			b.g.TransparentZeroOrMoreByNode(listNodeKind, "", defRule),
		)).
		build()
}

func (b *dslGrammarBuilder) prattPrefixDef() Rule {
	return b.g.sequence(NodePrattOperatorDef, "PREFIX").
		rule(b.prattOperatorTarget()).
		expectVirtualInRule(TokChainSeparator).
		expectToken(NodeParseNodeName, TokIdentifier).
		expectVirtualInRule(TokKWPrecedence).
		expectToken(NodePrattRightPrecedenceValue, TokInteger).
		expectVirtualInRule(TokSemicolon).
		build()
}

func (b *dslGrammarBuilder) prattPostfixDef() Rule {
	return b.g.sequence(NodePrattOperatorDef, "POSTFIX").
		rule(b.prattOperatorTarget()).
		expectVirtualInRule(TokChainSeparator).
		expectToken(NodeParseNodeName, TokIdentifier).
		expectVirtualInRule(TokKWPrecedence).
		expectToken(NodePrattLeftPrecedenceValue, TokInteger).
		expectVirtualInRule(TokSemicolon).
		build()
}

func (b *dslGrammarBuilder) prattInfixDef() Rule {
	return b.g.sequence(NodePrattOperatorDef, "INFIX").
		rule(b.prattOperatorTarget()).
		expectVirtualInRule(TokChainSeparator).
		expectToken(NodeParseNodeName, TokIdentifier).
		expectVirtualInRule(TokKWPrecedence).
		expectToken(NodePrattLeftPrecedenceValue, TokInteger).
		expectToken(NodePrattRightPrecedenceValue, TokInteger).
		expectVirtualInRule(TokSemicolon).
		build()
}

func (b *dslGrammarBuilder) prattOperatorTarget() Rule {
	return b.g.expectToken(NodeParseSymbolReference, TokIdentifier)
}

func (b *dslGrammarBuilder) prattImplicitBlock() Rule {
	return b.g.sequence(NodePrattImplicit, "").
		expectToken(NodePrattKeyword, TokKWImplicit).
		rule(b.g.NestByNode(
			NodePrattImplicitBody,
			TokBraceOpen,
			TokBraceClose,
			b.prattImplicitDef(),
		)).
		build()
}

func (b *dslGrammarBuilder) prattImplicitDef() Rule {
	return b.g.sequence(NodePrattImplicitDef, "").
		expectVirtualInRule(TokChainSeparator).
		expectToken(NodeParseNodeName, TokIdentifier).
		expectVirtualInRule(TokKWPrecedence).
		expectToken(NodePrattLeftPrecedenceValue, TokInteger).
		expectToken(NodePrattRightPrecedenceValue, TokInteger).
		expectVirtualInRule(TokSemicolon).
		build()
}

// ----------------------------------------------------------- PARSE SECTION

func (b *dslGrammarBuilder) parseSection() Rule {
	return b.g.block(
		NodeParseSection,
		NodeParseKeyword,
		TokKWParse,
		b.parseSectionBody(),
	)
}

func (b *dslGrammarBuilder) parseSectionBody() Rule {
	return b.g.sequence(NodeParseSectionBody, "").
		optionalRule(b.parseIgnoreSection()).
		rule(b.parseRuleList()).
		build()
}

func (b *dslGrammarBuilder) parseIgnoreSection() Rule {
	blockRule := b.g.block(
		NodeParseIgnoreSection,
		NodeParseIgnoreKeyword,
		TokKWIgnore,
		b.parseIgnoreRoleList(),
	)
	return b.g.rb.Rule.RecoverSync(blockRule, TokBraceClose)
}

func (b *dslGrammarBuilder) parseIgnoreRoleList() Rule {
	return b.g.TransparentZeroOrMoreByNode(
		NodeParseIgnoreRole,
		"LIST",
		b.parseIgnoreRole(),
	)
}

func (b *dslGrammarBuilder) parseIgnoreRole() Rule {
	return b.g.expectToken(NodeParseIgnoreRole, TokIdentifier)
}

func (b *dslGrammarBuilder) parseRuleList() Rule {
	parseRuleWithRecovery := b.g.rb.Rule.RecoverSync(b.parseRule(), TokSemicolon)
	return b.g.TransparentZeroOrMoreByNode(NodeParseRule, "LIST", parseRuleWithRecovery)
}

func (b *dslGrammarBuilder) parseRule() Rule {
	return b.g.sequence(NodeParseRule, "").
		expectToken(NodeParseRuleName, TokIdentifier).
		expectVirtualInRule(TokChainSeparator).
		optionalToken(NodeRuleModifierTransparent, TokKWTransparent).
		expectToken(NodeParseNodeName, TokIdentifier).
		optionalRule(b.syncModifier()).
		rule(b.g.NestByNode(
			NodeParseRuleBody,
			TokBraceOpen,
			TokBraceClose,
			b.g.rb.Rule.Required(b.parseRuleExpr(), "rule expression needs at least one expression"),
		)).
		optionalRule(b.g.expectVirtualInRule(NodeParseRule, TokSemicolon)).
		build()
}

func (b *dslGrammarBuilder) parseRuleExpr() Rule {
	cfg := rule.PrattConfig[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
		Primary: b.parseSegment(),

		PrefixRuleOps: []rule.PrattPrefixRuleOp[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
			{
				RightBP:  40,
				NodeKind: NodeParseModifierPredict,
				Rule:     b.predictModifier(),
			},
		},

		PostfixOps: []rule.PrattPostfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			b.g.PostfixOp(TokOptional, 30, NodeParseOptional),
			b.g.PostfixOp(TokStar, 30, NodeParseStar),
			b.g.PostfixOp(TokPlus, 30, NodeParsePlus),
		},

		PostfixRuleOps: []rule.PrattPostfixRuleOp[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
			b.g.PostfixRuleOp(30, NodeRepetition, b.patternRepetition()),
		},

		InfixOps: []rule.PrattInfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			b.g.InfixOp(TokPipe, 10, 9, NodeParseAlternation),
		},

		ImplicitInfix: &rule.PrattImplicitInfix[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			LeftBP:   20,
			RightBP:  19,
			NodeKind: NodeParseConcat,
		},
	}

	return b.g.rb.Pratt.Expression(VirtualGrammarIDToGrammarID(VirtualParseExpression), cfg)
}

func (b *dslGrammarBuilder) parseSegment() Rule {
	return b.g.ChoiceByNode(NodeParseSegment,
		b.virtualMapping(),
		b.nestMapping(),
		b.parseGroup(),
		b.identifierMapping(),
	)
}

func (b *dslGrammarBuilder) identifierMapping() Rule {
	groupRule := b.g.NestByNode(
		NodeParseGroup,
		TokParenOpen,
		TokParenClose,
		b.g.rb.Rule.Reference("PARSE EXPR REF", VirtualGrammarIDToGrammarID(VirtualParseExpression)),
	)

	tokenRefRule := b.g.expectToken(NodeParseTokenReference, TokIdentifier)

	tailChoice := b.g.ChoiceByNodeWithSuffix(NodeParseSegment, "TAIL_CHOICE", groupRule, tokenRefRule)

	tailSeq := b.g.sequence(NodeParseSegment, "TAIL").
		expectVirtualInRule(TokAssignment).
		rule(tailChoice).
		build()

	return b.g.sequence(NodeParseSegment, "IDENT_MAPPING_OR_REF").
		expectToken(NodeParseSymbolReference, TokIdentifier).
		optionalRule(tailSeq).
		build()
}

func (b *dslGrammarBuilder) virtualMapping() Rule {
	return b.g.sequence(NodeParseOpSuppress, "").
		expectVirtualInRule(TokKWVirtual).
		expectToken(NodeParseTokenReference, TokIdentifier).
		build()
}

func (b *dslGrammarBuilder) nestMapping() Rule {
	return b.g.sequence(NodeParseOpNest, "").
		expectVirtualInRule(TokKWNest).
		expectToken(NodeParseNestOpenToken, TokIdentifier).
		expectToken(NodeParseNestCloseToken, TokIdentifier).
		optionalRule(b.syncModifier()).
		rule(b.g.rb.Rule.Choice(
			"DUMMY CHOICE NEST",
			b.g.expectToken(NodeParseExpressionReference, TokIdentifier),
			b.g.NestByNode(
				NodeParseNestBody,
				TokBraceOpen,
				TokBraceClose,
				b.g.rb.Rule.Required(
					b.g.rb.Rule.Reference("PARSE EXPR REF", VirtualGrammarIDToGrammarID(VirtualParseExpression)),
					"rule expression needs at least one expression",
				),
			))).
		build()
}

func (b *dslGrammarBuilder) parseGroup() Rule {
	return b.g.NestByNode(
		NodeParseGroup,
		TokParenOpen,
		TokParenClose,
		b.g.rb.Rule.Reference("PARSE EXPR REF", VirtualGrammarIDToGrammarID(VirtualParseExpression)),
	)
}

// ----------------------------------------------------------- GENERAL

func (b *dslGrammarBuilder) syncModifier() Rule {
	return b.g.sequence(NodeRuleModifierSync, "").
		expectVirtualInRule(TokKWSync).
		rule(b.g.NestByNode(
			NodeSyncBlock,
			TokParenOpen,
			TokParenClose,
			b.g.TransparentZeroOrMoreByNode(NodeSyncToken, "LIST", b.g.expectToken(NodeSyncToken, TokIdentifier)),
		)).
		build()
}

// --- Lookahead (Predict) ---

func (b *dslGrammarBuilder) predictModifier() Rule {
	return b.g.sequence(NodeParseModifierPredict, "").
		expectVirtualInRule(TokKWPredict).
		rule(b.g.NestByNode(
			NodePredictLookaheadList,
			TokParenOpen,
			TokParenClose,
			b.g.TransparentZeroOrMoreByNode(NodePredictLookahead, "ITEMS", b.predictLookahead()),
		)).
		build()
}

func (b *dslGrammarBuilder) predictLookahead() Rule {
	return b.g.sequence(NodePredictLookahead, "").
		expectToken(NodePredictOffset, TokInteger).
		expectVirtualInRule(TokAssignment).
		expectToken(NodePredictToken, TokIdentifier).
		optionalRule(b.g.expectVirtualInRule(NodePredictLookahead, TokComma)).
		build()
}

func (b *dslGrammarBuilder) patternRefReference() Rule {
	return b.g.rb.Rule.Reference(
		syntaxa.GrammarLabel("VAR REF PROXY"),
		LangSpecGrammarIDFromNode(NodePatternRef),
	)
}

func (b *dslGrammarBuilder) declarePatternRef() {
	r := b.g.expectToken(NodePatternRef, TokIdentifier)
	b.g.rb.Rule.Define(r)
}

func (b *dslGrammarBuilder) charLiteral() Rule {
	return b.g.expectToken(NodeCharLiteral, TokCharLiteral)
}

// ----------------------------------------------------------- PATTERN HELPERS

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

func buildBlockCommentOpenPattern(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	return f.Sequence(f.Literal('/'), f.Literal('*'))
}

func buildBlockCommentClosePattern(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	return f.Sequence(f.Literal('*'), f.Literal('/'))
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

	standardEscape := f.Sequence(escapeTrigger, escapedChar)
	unicodeEscape := buildUnicodeEscapePattern(f)

	charContent := f.AnyOf(normalChar, standardEscape, unicodeEscape)

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

	standardEscape := f.Sequence(escapeTrigger, escapedChar)
	unicodeEscape := buildUnicodeEscapePattern(f)

	body := f.AnyOf(normalChar, standardEscape, unicodeEscape).Star()

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

func buildUnicodeEscapePattern(f *pattern.RegulaASTFactory[rune]) pattern.RegulaAST[rune] {
	hexDigit := f.Class(
		f.Range('0', '9'),
		f.Range('a', 'f'),
		f.Range('A', 'F'),
	)

	shortHex := f.Sequence(f.Literal('\\', 'u'), hexDigit.Repeat(4, 4))
	longHex := f.Sequence(f.Literal('\\', 'U'), hexDigit.Repeat(8, 8))

	return f.AnyOf(shortHex, longHex)
}
