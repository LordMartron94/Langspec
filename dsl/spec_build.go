package dsl

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
	TokVarRef // $

	// -- Punctuation & Operators --
	TokDashes
	TokBraceOpen
	TokBraceClose
	TokParenOpen
	TokParenClose
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
	TokKWRef

	TokKWTrue
	TokKWFalse
	TokKWLocal
	TokKWVirtual
	TokKWNest
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
	NodePatternDefName
	NodeVarRef
	NodeVarRefToken
	NodeVarRefTarget
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
	NodeIdentifier

	NodeLocalVariable

	// -- Parse Section --
	NodeParseSection
	NodeParseKeyword
	NodeParseRule

	NodeParseRuleName
	NodeParseNodeName
	NodeParseRuleBody
	NodeParseOpSuppress
	NodeParseOpEmit
	NodeParseOpRef
	NodeParseAlternation
	NodeParseConcat
	NodeParseOptional
	NodeParseSegment
	NodeParseGroup
	NodeParseTokenReference
	NodeParseRuleReference

	NodeParseOpNest
	NodeParseNestOpenToken
	NodeParseNestCloseToken
	NodeParseNestBody

	NodeParseStar
	NodeParsePlus
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
		{TokComma, "punctuation.separator.comma", ",", 0},
		{TokPlus, "keyword.operator.plus", "+", 0},
		{TokNegation, "keyword.operator.negation", "!", 0},
		{TokAssignment, "keyword.operator.assignment", ":", 0},
		{TokChainSeparator, "punctuation.separator.chain", "->", 0},
		{TokEqualsOperator, "keyword.operator.assignment", "=", 0},
		{TokOptional, "keyword.operator.optional", "?", 0},
		{TokVarRef, "keyword.operator.variable", "$", 0},
		{TokRange, "keyword.operator.range", "..", 0},
		{TokStar, "keyword.operator.star", "*", 0},
		{TokKWLSpec, "keyword.declaration", "lspec", 2},
		{TokKWPragma, "keyword.pragma", "PRAGMA", 2},
		{TokKWTool, "keyword.tool", "tool", 2},
		{TokKWLex, "keyword.declaration.lex", "LEX", 2},
		{TokKWPattern, "keyword.declaration.pattern", "PATTERN", 2},
		{TokKWTrue, "constant.language.boolean", "true", 2},
		{TokKWFalse, "constant.language.boolean", "false", 2},
		{TokKWLocal, "keyword.modifier.local", "local", 2},
		{TokKWParse, "keyword.declaration.parse", "PARSE", 2},
		{TokKWRef, "keyword.control.reference", "ref", 2},
		{TokKWVirtual, "keyword.operator.virtual", "virtual", 2},
		{TokKWNest, "keyword.operator.nest", "nest", 2},
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
	b.declareVarRef()

	return b.g.RootByNode(NodeProgram, false,
		b.g.rb.Rule.Required(b.header(), "must have header"),
		b.g.rb.Rule.OptionalPrefix(b.pragmaSection(), TokKWPragma),
		b.g.rb.Rule.OptionalPrefix(b.patternSection(), TokKWPattern),
		b.g.rb.Rule.Required(b.lexSection(), "must have lex ruleset"),
		b.g.rb.Rule.Required(b.parseSection(), "must have parse ruleset"),
		b.g.expectVirtual(VirtualEOF, TokEOF),
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
		rule(b.g.expectOneOf(NodePragmaValue, TokIdentifier, TokStringLiteral, TokKWTrue, TokKWFalse)).
		expectVirtualInRule(TokSemicolon).
		build(), TokSemicolon)
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
		optionalToken(NodeLocalVariable, TokKWLocal).
		expectToken(NodePatternDefName, TokIdentifier).
		expectVirtualInRule(TokAssignment).
		requiredRule(b.patternExpr(), "pattern definition must have an expression").
		expectVirtualInRule(TokSemicolon).
		build(), TokSemicolon)
}

func (b *dslGrammarBuilder) patternExpr() Rule {
	cfg := rule.PrattConfig[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
		// PRIMARY: Only the base units (literals, groups, var refs)
		Primary: b.patternSegment(),

		// PREFIX: Bindings that happen BEFORE the expression
		PrefixOps: []rule.PrattPrefixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			b.g.PrefixOp(TokNegation, 40, NodePatternNegation), // High precedence
		},

		// POSTFIX: Bindings that happen AFTER the expression (Star, Plus)
		PostfixOps: []rule.PrattPostfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			b.g.PostfixOp(TokStar, 30, NodePatternStar),
			b.g.PostfixOp(TokPlus, 30, NodePatternPlus),
			b.g.PostfixOp(TokOptional, 30, NodePatternOptional),
		},

		// POSTFIX RULES: Composite postfix bindings that require full sub-rule execution
		PostfixRuleOps: []rule.PrattPostfixRuleOp[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
			b.g.PostfixRuleOp(TokBraceOpen, 30, NodeRepetition, b.patternRepetition()),
		},

		// INFIX: Bindings BETWEEN expressions
		InfixOps: []rule.PrattInfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			b.g.InfixOp(TokPipe, 10, 9, NodePatternAlternation),
		},

		// IMPLICIT: Handling 'a' 'b' (Concat)
		ImplicitInfix: &rule.PrattImplicitInfix[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			LeftBP:   20,
			RightBP:  19,
			NodeKind: NodePatternConcat,
		},

		RecoveryTokens: []LangSpecLexerTokenType{TokSemicolon},
	}

	return b.g.rb.Pratt.Expression(VirtualGrammarIDToGrammarID(VirtualPatternExpression), cfg)
}

func (b *dslGrammarBuilder) patternSegment() Rule {
	rangeRule := b.g.rb.Rule.Predict(
		b.patternRange(),
		func(ctx *syntaxa.SelectRuleContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole]) bool {
			return ctx.Peek(1).Token == TokRange
		},
	)

	return b.g.ChoiceByNode(NodePatternSegment,
		b.varRefReference(),
		rangeRule,
		b.charLiteral(),
		b.g.expectToken(NodePatternAny, TokDot),
		b.g.NestByNode(NodePatternGroup, TokParenOpen, TokParenClose,
			b.g.rb.Rule.Reference("PATTERN EXPR REF", VirtualGrammarIDToGrammarID(VirtualPatternExpression))),
	)
}

func (b *dslGrammarBuilder) patternRange() Rule {
	return b.g.sequence(NodePatternRange, "").
		expectToken(NodeCharLiteral, TokCharLiteral).
		expectVirtualInRule(TokRange).
		expectToken(NodeCharLiteral, TokCharLiteral).
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
	return b.g.ChoiceByNode(
		NodeRepetitionBounds,
		b.rangedRepetition(),
		b.exactRepetition(),
	)
}

func (b *dslGrammarBuilder) rangedRepetition() Rule {
	return b.g.sequence(NodeRepetitionBounds, "RANGED").
		optionalToken(NodeRepetitionMin, TokInteger).
		expectVirtualInRule(TokComma).
		optionalToken(NodeRepetitionMax, TokInteger).
		build()
}

func (b *dslGrammarBuilder) exactRepetition() Rule {
	return b.g.sequence(NodeRepetitionBounds, "EXACT").
		expectToken(NodeRepetitionMin, TokInteger).
		build()
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
			b.varRefReference(),
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

// ----------------------------------------------------------- PARSE SECTION

func (b *dslGrammarBuilder) parseSection() Rule {
	return b.g.block(
		NodeParseSection,
		NodeParseKeyword,
		TokKWParse,
		b.parseRuleList(),
	)
}

func (b *dslGrammarBuilder) parseRuleList() Rule {
	parseRuleWithRecovery := b.g.rb.Rule.RecoverSync(b.parseRule(), TokSemicolon)
	return b.g.TransparentZeroOrMoreByNode(NodeParseRule, "LIST", parseRuleWithRecovery)
}

func (b *dslGrammarBuilder) parseRule() Rule {
	return b.g.sequence(NodeParseRule, "").
		expectToken(NodeParseRuleName, TokIdentifier).
		expectVirtualInRule(TokChainSeparator).
		expectToken(NodeParseNodeName, TokIdentifier).
		rule(b.g.NestByNode(
			NodeParseRuleBody,
			TokBraceOpen,
			TokBraceClose,
			b.parseRuleExpr(),
		)).
		optionalRule(b.g.expectVirtualInRule(NodeParseRule, TokSemicolon)).
		build()
}

func (b *dslGrammarBuilder) parseRuleExpr() Rule {
	cfg := rule.PrattConfig[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
		Primary: b.parseSegment(),

		PostfixOps: []rule.PrattPostfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			b.g.PostfixOp(TokOptional, 30, NodeParseOptional),
			b.g.PostfixOp(TokStar, 30, NodeParseStar),
			b.g.PostfixOp(TokPlus, 30, NodeParsePlus),
		},

		PostfixRuleOps: []rule.PrattPostfixRuleOp[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole, LangSpecLexerState, LangSpecParserNodeKind]{
			b.g.PostfixRuleOp(TokBraceOpen, 30, NodeRepetition, b.patternRepetition()),
		},

		InfixOps: []rule.PrattInfixOp[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			b.g.InfixOp(TokPipe, 10, 9, NodeParseAlternation),
		},

		ImplicitInfix: &rule.PrattImplicitInfix[LangSpecLexerTokenType, LangSpecParserNodeKind]{
			LeftBP:   20,
			RightBP:  19,
			NodeKind: NodeParseConcat,
		},

		RecoveryTokens: []LangSpecLexerTokenType{TokSemicolon, TokBraceClose},
	}

	return b.g.rb.Pratt.Expression(VirtualGrammarIDToGrammarID(VirtualParseExpression), cfg)
}

func (b *dslGrammarBuilder) parseSegment() Rule {
	return b.g.ChoiceByNode(NodeParseSegment,
		b.emitMapping(),
		b.refMapping(),
		b.virtualMapping(),
		b.nestMapping(),
		b.g.expectToken(NodeIdentifier, TokIdentifier),
		b.parseGroup(),
	)
}

func (b *dslGrammarBuilder) emitMapping() Rule {
	emit := b.g.sequence(NodeParseOpEmit, "").
		expectToken(NodeParseNodeName, TokIdentifier).
		expectVirtualInRule(TokAssignment).
		expectToken(NodeParseTokenReference, TokIdentifier).
		build()

	return b.g.rb.Rule.Predict(
		emit,
		func(ctx *syntaxa.SelectRuleContext[rune, LangSpecLexerTokenType, LangSpecLexerTokenRole]) bool {
			return ctx.Peek(0).Token == TokIdentifier && ctx.Peek(1).Token == TokAssignment
		},
	)
}

func (b *dslGrammarBuilder) refMapping() Rule {
	return b.g.sequence(NodeParseOpRef, "").
		expectVirtualInRule(TokKWRef).
		expectToken(NodeParseRuleReference, TokIdentifier).
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
		rule(b.g.NestByNode(
			NodeParseNestBody,
			TokBraceOpen,
			TokBraceClose,
			b.g.rb.Rule.Reference("PARSE EXPR REF", VirtualGrammarIDToGrammarID(VirtualParseExpression)),
		)).
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

/*
varRefReference returns the late-bound DAG proxy.
*/
func (b *dslGrammarBuilder) varRefReference() Rule {
	return b.g.rb.Rule.Reference(
		syntaxa.GrammarLabel("VAR REF PROXY"),
		LangSpecGrammarIDFromNode(NodeVarRef),
	)
}

func (b *dslGrammarBuilder) declareVarRef() {
	r := b.g.expectPairWithChildNodes(NodeVarRef, NodeVarRefToken, NodeVarRefTarget, TokVarRef, TokIdentifier)
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
