package spec

import (
	"autarch/pattern"
	"foundation/domain"
	"lexarch"
	"syntaxa"
	"syntaxa/rule"
)

// ----------------------------------------------------------- LEXER / PARSER ENUMS

// LangSpecLexerState is the lexer mode key for the LangSpec meta-language (.lspec).
type LangSpecLexerState = string

const LangSpecLexerStateInitial LangSpecLexerState = "INITIAL"

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
	TokColon // :
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

	TokKWState

	TokKWPush
	TokKWPop
	TokKWSet

	TokKWPair
	TokPairReference
	TokParameter

	TokKWRule
	TokKWTemplate
	TokKWCall
	TokKWImport
	TokKWExport
	TokKWAs
	TokKWUsing
	TokKWEmbed

	TokTypeToken
	TokTypeRule
	TokTypePrattExpr
	TokTypePair
	TokTypeNode
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
	NodeStateList
	NodeStateKeyword
	NodeStateDefinitionBody
	NodeStateDefinitionList
	NodeStateDefinition
	NodeLexRuleStateMutation
	NodeStateMutationPush
	NodeStateMutationPop
	NodeStateMutationSet
	NodeStateMutationArgumentList
	NodeStateMutationPopAmount
	NodeStateReference

	NodeLexRulePatternUsing
	NodeModuleReference
	NodePatternExternalPatternReference

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

	NodeParsePair
	NodeParsePairIdentifier
	NodeParseNestPairRef

	NodeParseTemplate
	NodeParseTemplateIdentifier
	NodeParseTemplateParameter
	NodeParseTemplateParameterIdentifier
	NodeParseTemplateParameterType
	NodeParseTemplateParameterReference
	NodeParseTemplateBody
	NodeParseTemplateParameterList
	NodeParseTemplateSignature

	NodeParseTemplateCallKeyword
	NodeParseTemplateCallArgs
	NodeParseTemplateCallArgument

	NodeParseTemplateReference

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

	// Dummy
	NodeDummyChoiceParseBlock

	// IMPORT & EXPORT
	NodeImportSection
	NodeImportKeyword
	NodeImportEmbed

	NodeImportList
	NodeImportDefinition

	NodeImportPath
	NodeImportAlias

	NodeExported
	NodeParseBlock
	NodeParseEmbedStatement
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
		{TokColon, "keyword.operator.assignment", ":", 0},
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
		{TokKWLocal, "storage.modifier.local", "local", 2},
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
		{TokKWState, "keyword.declaration.state", "state", 2},
		{TokKWTransparent, "keyword.operator.transparent", "transparent", 2},
		{TokKWSync, "keyword.operator.sync", "sync", 2},
		{TokKWPredict, "keyword.operator.predict", "predict", 2},
		{TokKWPush, "keyword.operator.push", "push", 2},
		{TokKWPop, "keyword.operator.pop", "pop", 2},
		{TokKWSet, "keyword.operator.set", "set", 2},
		{TokKWPair, "keyword.declaration.pair", "pair", 2},
		{TokKWTemplate, "keyword.declaration.template", "template", 2},
		{TokKWCall, "keyword.operator.call", "call", 2},
		{TokKWRule, "keyword.declaration.rule", "rule", 2},
		{TokKWImport, "keyword.declaration.import", "IMPORT", 2},
		{TokKWExport, "storage.modifier.export", "export", 2},
		{TokKWAs, "keyword.control.import.as", "as", 2},
		{TokKWUsing, "keyword.operator.using", "using", 2},
		{TokKWEmbed, "keyword.operator.embed", "embed", 2},

		// Types
		{TokTypeToken, "support.type.token", "Token", 2},
		{TokTypeRule, "support.type.rule", "Rule", 2},
		{TokTypeNode, "support.type.node", "Node", 2},
		{TokTypePair, "support.type.pair", "Pair", 2},
		{TokTypePrattExpr, "support.type.pratt-expression", "PrattExpr", 2},
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

		DefineToken(TokPairReference).
			Scope("constant.language.token-pair-reference").
			HighPriority().
			PatternFromRegex(`@[a-zA-Z_][a-zA-Z0-9_\-]*`, f).
			Build(),

		DefineToken(TokParameter).
			Scope("variable.parameter").
			HighPriority().
			PatternFromRegex(`$[a-zA-Z_][a-zA-Z0-9_\-]*`, f).
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
	b.declareParseExpression()

	return b.g.RootByNode(NodeProgram, false,
		b.g.rb.Rule.Required(b.header(), "must have header"),
		b.g.rb.Rule.OptionalPrefix(b.pragmaSection(), lexarch.TokenKind(TokKWPragma)),
		b.g.rb.Rule.OptionalPrefix(b.importSection(), lexarch.TokenKind(TokKWImport)),
		b.g.rb.Rule.OptionalPrefix(b.patternSection(), lexarch.TokenKind(TokKWPattern)),
		b.g.rb.Rule.Required(b.lexSection(), "must have lex ruleset"),
		b.g.rb.Rule.OptionalPrefix(b.prattSection(), lexarch.TokenKind(TokKWPratt)),
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
	return b.g.rb.Rule.RecoverSync(blockRule, lexarch.TokenKind(TokSemicolon), lexarch.TokenKind(TokBraceClose))
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
	return b.g.rb.Rule.RecoverSync(blockRule, lexarch.TokenKind(TokSemicolon), lexarch.TokenKind(TokBraceClose))
}

func (b *dslGrammarBuilder) blockKey() Rule {
	optionalToolSegment := b.g.rb.Rule.Optional(
		b.g.sequence(NodePragmaBlockKeySegment, "").
			expectVirtualInRule(TokDot).
			expectToken(NodePragmaBlockKeySegment, TokIdentifier).
			build(),
	)
	return b.g.sequence(NodePragmaBlockKey, "").
		rule(b.g.expectOneOf(NodePragmaBlockKeyPrefix, TokKWTool, TokKWLSpec)).
		rule(optionalToolSegment).
		build()
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
		build(), lexarch.TokenKind(TokSemicolon))
}

func (b *dslGrammarBuilder) stringArray() Rule {
	elementListRule := b.g.rb.Rule.TransparentSequence(
		"STRING_ARRAY_ELEMENT_LIST",
		b.g.rb.Token.Expect(LangSpecGrammarIDFromNode(NodeStringLiteral), NodeStringLiteral, lexarch.TokenKind(TokStringLiteral)),
		b.g.rb.Rule.TransparentZeroOrMore(
			"STRING_ARRAY_ELEMENT_LIST_TAIL",
			b.g.rb.Rule.TransparentSequence(
				"STRING_ARRAY_ELEMENT_LIST_TAIL_CONTENT",
				b.g.rb.Token.ExpectVirtual("COMMA", lexarch.TokenKind(TokComma)),
				b.g.rb.Token.Expect(LangSpecGrammarIDFromNode(NodeStringLiteral), NodeStringLiteral, lexarch.TokenKind(TokStringLiteral)),
			),
		),
	)

	return b.g.rb.Rule.Nest(
		LangSpecGrammarIDFromNode(NodeStringArray), NodeStringArray,
		lexarch.TokenKind(TokBracketOpen), lexarch.TokenKind(TokBracketClose),
		elementListRule,
	)
}

// ----------------------------------------------------------- IMPORT SECTION

func (b *dslGrammarBuilder) importSection() Rule {
	blockRule := b.g.block(
		NodeImportSection,
		NodeImportKeyword,
		TokKWImport,
		b.importList(),
	)
	return b.g.rb.Rule.RecoverSync(blockRule, lexarch.TokenKind(TokSemicolon), lexarch.TokenKind(TokBraceClose))
}

func (b *dslGrammarBuilder) importList() Rule {
	return b.g.TransparentZeroOrMoreByNode(NodeImportDefinition, "LIST", b.importDefinition())
}

func (b *dslGrammarBuilder) importDefinition() Rule {
	definitionRule := b.g.sequence(
		NodeImportDefinition, "",
	).
		optionalRule(b.g.expectToken(NodeImportEmbed, TokKWEmbed)).
		expectToken(NodeImportPath, TokStringLiteral).
		expectVirtualInRule(TokKWAs).
		expectToken(NodeImportAlias, TokIdentifier).
		expectVirtualInRule(TokSemicolon).
		build()

	return b.g.rb.Rule.RecoverSync(definitionRule, lexarch.TokenKind(TokSemicolon))
}

// ----------------------------------------------------------- PATTERN SECTION

func (b *dslGrammarBuilder) patternSection() Rule {
	blockRule := b.g.block(
		NodePatternSection,
		NodePatternKeyword,
		TokKWPattern,
		b.patternDefinitionList(),
	)
	return b.g.rb.Rule.RecoverSync(blockRule, lexarch.TokenKind(TokSemicolon), lexarch.TokenKind(TokBraceClose))
}

func (b *dslGrammarBuilder) patternDefinitionList() Rule {
	return b.g.TransparentZeroOrMoreByNode(NodePatternDefinition, "LIST", b.patternDefinition())
}

func (b *dslGrammarBuilder) patternDefinition() Rule {
	optionalPrefixRule := b.g.rb.Rule.Optional(
		b.g.rb.Rule.Choice(
			"PREFIX_DUMMY_RULE_PATTERNS",
			b.g.expectToken(NodeLocalVariable, TokKWLocal),
			b.g.expectToken(NodeExported, TokKWExport),
		),
	)

	return b.g.rb.Rule.RecoverSync(b.g.sequence(NodePatternDefinition, "").
		rule(optionalPrefixRule).
		expectToken(NodePatternDefName, TokIdentifier).
		expectVirtualInRule(TokColon).
		requiredRule(b.patternExpr(), "pattern definition must have an expression").
		expectVirtualInRule(TokSemicolon).
		build(), lexarch.TokenKind(TokSemicolon))
}

func (b *dslGrammarBuilder) patternExpr() Rule {
	cfg := rule.PrattConfig[LangSpecParserNodeKind]{
		Primary: b.patternSegment(),

		PrefixOps: []rule.PrattPrefixOp[LangSpecParserNodeKind]{
			b.g.PrefixOp(TokNegation, 40, NodePatternNegation),
		},

		PostfixOps: []rule.PrattPostfixOp[LangSpecParserNodeKind]{
			b.g.PostfixOp(TokStar, 30, NodePatternStar),
			b.g.PostfixOp(TokPlus, 30, NodePatternPlus),
			b.g.PostfixOp(TokOptional, 30, NodePatternOptional),
		},

		PostfixRuleOps: []rule.PrattPostfixRuleOp[LangSpecParserNodeKind]{
			b.g.PostfixRuleOp(30, NodeRepetition, b.patternRepetition()),
		},

		InfixOps: []rule.PrattInfixOp[LangSpecParserNodeKind]{
			b.g.InfixOp(TokPipe, 10, 9, NodePatternAlternation),
			b.g.InfixOp(TokRange, 60, 61, NodePatternRange),
		},

		ImplicitInfix: &rule.PrattImplicitInfix[LangSpecParserNodeKind]{
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
		b.lexStateBlockList(),
	)
}

func (b *dslGrammarBuilder) lexStateBlockList() Rule {
	return b.g.TransparentZeroOrMoreByNode(NodeStateList, "BLOCKS", b.stateList())
}

func (b *dslGrammarBuilder) stateList() Rule {
	return b.g.sequence(
		NodeStateList, "",
	).expectToken(NodeStateKeyword, TokKWState).
		rule(b.stateDefinitions()).
		rule(b.g.NestByNode(
			NodeStateDefinitionBody,
			TokBraceOpen,
			TokBraceClose,
			b.lexRuleList(),
		)).
		build()
}

func (b *dslGrammarBuilder) stateDefinitions() Rule {
	// IDENTIFIER (, IDENTIFIER)*
	return b.g.sequence(NodeStateDefinitionList, "").
		expectToken(NodeStateDefinition, TokIdentifier).
		rule(
			b.g.TransparentZeroOrMoreByNode(
				NodeStateDefinition, "TAIL",
				b.g.rb.Rule.TransparentSequence(
					LangSpecGrammarIDFromNodeWithSuffix(NodeStateDefinition, "TAIL CONTENT"),
					b.g.expectVirtual(VirtualComma, TokComma),
					b.g.expectToken(NodeStateDefinition, TokIdentifier),
				),
			),
		).
		build()
}

func (b *dslGrammarBuilder) lexRuleList() Rule {
	lexRuleWithRecovery := b.g.rb.Rule.RecoverSync(b.lexRule(), lexarch.TokenKind(TokSemicolon))
	return b.g.TransparentZeroOrMoreByNode(NodeLexRule, "LIST", lexRuleWithRecovery)
}

func (b *dslGrammarBuilder) lexRule() Rule {
	return b.g.sequence(NodeLexRule, "").
		optionalToken(NodeLexRulePriority, TokInteger).
		expectToken(NodeLexRuleTokenName, TokIdentifier).
		expectVirtualInRule(TokChainSeparator).
		expectToken(NodeLexRuleRole, TokIdentifier).
		expectVirtualInRule(TokColon).
		rule(b.g.ChoiceByNode(NodeLexRulePattern,
			b.patternRefReference(),
			b.g.expectToken(NodeLexRulePattern, TokRegexLiteral),
			b.usingPattern("LEX"),
		)).
		optionalRule(b.stateMutation()).
		optionalRule(b.metaSection()).
		expectVirtualInRule(TokSemicolon).
		build()
}

func (b *dslGrammarBuilder) usingPattern(suffix string) Rule {
	// Suffix distinguishes LEX vs NEST vs PARSE_SEGMENT sites. sequence(..., "") would memoize
	// one NodeLexRulePatternUsing production and the first optional segmentTemplateCallTail would
	// wire all three contexts to the same PARSE_TEMPLATE_CALL_ARGS_* variant (wrong + V_PAR003).
	return b.g.sequence(
		NodeLexRulePatternUsing, suffix,
	).expectVirtualInRule(TokKWUsing).
		expectToken(NodeModuleReference, TokIdentifier).
		expectVirtualInRule(TokDot).
		expectToken(NodePatternExternalPatternReference, TokIdentifier).
		optionalRule(b.segmentTemplateCallTail("USING_" + suffix)).
		build()
}

// ----------------------------------------------------------- LEX: STATE MUTATIONS

func (b *dslGrammarBuilder) stateMutation() Rule {
	// Inner choice must use ChoiceByNodeWithSuffix: NestByNode and ChoiceByNode share the same
	// LangSpecGrammarIDFromNode(NodeLexRuleStateMutation) memo key; without a suffix the choice is
	// memoized first and NestByNode returns it, dropping the [ ] bracket nest from the grammar IR.
	return b.g.NestByNode(
		NodeLexRuleStateMutation,
		TokBracketOpen,
		TokBracketClose,
		b.g.ChoiceByNodeWithSuffix(NodeLexRuleStateMutation, "KIND",
			b.stateMutationPush(),
			b.stateMutationPop(),
			b.stateMutationSet(),
		),
	)
}

func (b *dslGrammarBuilder) stateMutationPush() Rule {
	return b.g.sequence(NodeStateMutationPush, "").
		expectToken(NodeLexRuleStateMutation, TokKWPush).
		requiredRule(b.g.NestByNodeWithSuffix(
			NodeStateMutationArgumentList, "ARGS",
			TokParenOpen,
			TokParenClose,
			b.stateListArgs(),
		), "push requires (State, ...)").
		build()
}

func (b *dslGrammarBuilder) stateMutationPop() Rule {
	return b.g.sequence(NodeStateMutationPop, "").
		expectToken(NodeLexRuleStateMutation, TokKWPop).
		requiredRule(b.g.NestByNodeWithSuffix(
			NodeStateMutationArgumentList, "POP",
			TokParenOpen,
			TokParenClose,
			b.g.expectToken(NodeStateMutationPopAmount, TokInteger),
		), "pop requires (N)").
		build()
}

func (b *dslGrammarBuilder) stateMutationSet() Rule {
	return b.g.sequence(NodeStateMutationSet, "").
		expectToken(NodeLexRuleStateMutation, TokKWSet).
		requiredRule(b.g.NestByNodeWithSuffix(
			NodeStateMutationArgumentList, "ARGS",
			TokParenOpen,
			TokParenClose,
			b.stateListArgs(),
		), "set requires (State, ...)").
		build()
}

func (b *dslGrammarBuilder) stateMutationArgListTailSegment() Rule {
	// Memoized: TransparentZeroOrMoreByNode evaluates its element rule before checking its own memo cache,
	// so a fresh TransparentSequence would register duplicate "… TAIL_CONTENT" context boundaries.
	return b.g.memoize(LangSpecGrammarIDFromNodeWithSuffix(NodeStateMutationArgumentList, "TAIL_CONTENT"), func() Rule {
		return b.g.rb.Rule.TransparentSequence(
			LangSpecGrammarIDFromNodeWithSuffix(NodeStateMutationArgumentList, "TAIL_CONTENT"),
			b.g.expectVirtual(VirtualComma, TokComma),
			b.g.expectToken(NodeStateReference, TokIdentifier),
		)
	})
}

func (b *dslGrammarBuilder) stateListArgs() Rule {
	// Parses: IDENTIFIER (, IDENTIFIER)*
	return b.g.sequence(NodeStateMutationArgumentList, "LIST").
		expectToken(NodeStateReference, TokIdentifier).
		rule(
			b.g.TransparentZeroOrMoreByNode(
				NodeStateMutationArgumentList, "TAIL",
				b.stateMutationArgListTailSegment(),
			),
		).
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
	return b.g.rb.Rule.RecoverSync(blockRule, lexarch.TokenKind(TokSemicolon), lexarch.TokenKind(TokBraceClose))
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
		rule(b.parseBlockList()).
		build()
}

func (b *dslGrammarBuilder) parseIgnoreSection() Rule {
	blockRule := b.g.block(
		NodeParseIgnoreSection,
		NodeParseIgnoreKeyword,
		TokKWIgnore,
		b.parseIgnoreRoleList(),
	)
	return b.g.rb.Rule.RecoverSync(blockRule, lexarch.TokenKind(TokBraceClose))
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

func (b *dslGrammarBuilder) parseBlockList() Rule {
	return b.g.TransparentZeroOrMoreByNode(
		NodeDummyChoiceParseBlock, "LIST", b.parseBlock(),
	)
}

func (b *dslGrammarBuilder) parseBlock() Rule {
	parseRuleWithRecovery := b.g.rb.Rule.RecoverSync(b.parseRule(), lexarch.TokenKind(TokSemicolon))
	pairDeclaration := b.pairDeclaration()
	templateDeclaration := b.templateDeclaration()

	element := b.g.rb.Rule.Choice(
		LangSpecGrammarIDFromNodeWithSuffix(NodeDummyChoiceParseBlock, "CHOICE"),
		parseRuleWithRecovery,
		pairDeclaration,
		templateDeclaration,
	)

	return b.g.sequence(
		NodeParseBlock, "",
	).optionalRule(b.g.expectToken(NodeExported, TokKWExport)).
		rule(element).build()
}

func (b *dslGrammarBuilder) templateDeclaration() Rule {
	templateArgList := b.templateArgumentList()

	return b.g.sequence(
		NodeParseTemplate, "DECLARATION",
	).
		expectVirtualInRule(TokKWTemplate).
		expectToken(NodeParseTemplateIdentifier, TokIdentifier).
		rule(b.g.TransparentNestByNode(
			NodeParseTemplateSignature, "",
			TokParenOpen, TokParenClose,
			b.g.rb.Rule.Optional(templateArgList),
		)).
		rule(b.g.NestByNode(
			NodeParseTemplateBody,
			TokBraceOpen, TokBraceClose,
			b.parseExpressionReference(),
		)).
		build()
}

func (b *dslGrammarBuilder) templateArgumentList() Rule {
	templateArgument := b.templateArgument()

	return b.g.sequence(
		NodeParseTemplateParameterList, "",
	).
		rule(templateArgument).
		rule(
			b.g.rb.Rule.TransparentZeroOrMore(
				"TEMPLATE_ARGUMENT_LIST_TAIL",
				b.g.rb.Rule.TransparentSequence(
					"TEMPLATE_ARGUMENT_LIST_TAIL_CONTENT",
					b.g.rb.Token.ExpectVirtual("COMMA", lexarch.TokenKind(TokComma)),
					templateArgument,
				),
			),
		).
		build()
}

func (b *dslGrammarBuilder) templateArgument() Rule {
	return b.g.sequence(
		NodeParseTemplateParameter, "",
	).
		expectToken(NodeParseTemplateParameterIdentifier, TokParameter).
		expectVirtualInRule(TokColon).
		rule(b.g.expectOneOf(NodeParseTemplateParameterType, TokTypeToken, TokTypeNode, TokTypeRule, TokTypePair, TokTypePrattExpr)).
		build()
}

func (b *dslGrammarBuilder) pairDeclaration() Rule {
	declaration := b.g.sequence(NodeParsePair, "DECLARATION").
		expectVirtualInRule(TokKWPair).
		expectToken(NodeParsePairIdentifier, TokIdentifier).
		expectToken(NodeParseTokenReference, TokIdentifier).
		expectToken(NodeParseTokenReference, TokIdentifier).
		expectVirtualInRule(TokSemicolon).
		build()
	return b.g.rb.Rule.RecoverSync(declaration, lexarch.TokenKind(TokSemicolon))
}

func (b *dslGrammarBuilder) parseRule() Rule {
	return b.g.sequence(NodeParseRule, "").
		expectVirtualInRule(TokKWRule).
		expectToken(NodeParseRuleName, TokIdentifier).
		expectVirtualInRule(TokChainSeparator).
		optionalToken(NodeRuleModifierTransparent, TokKWTransparent).
		expectToken(NodeParseNodeName, TokIdentifier).
		optionalRule(b.syncModifier()).
		rule(b.g.NestByNode(
			NodeParseRuleBody,
			TokBraceOpen,
			TokBraceClose,
			b.g.rb.Rule.Required(b.parseExpressionReference(), "rule expression needs at least one expression"),
		)).
		optionalRule(b.g.expectVirtualInRule(NodeParseRule, TokSemicolon)).
		build()
}

func (b *dslGrammarBuilder) parseRuleExpr() Rule {
	cfg := rule.PrattConfig[LangSpecParserNodeKind]{
		Primary: b.parseSegment(),

		PrefixRuleOps: []rule.PrattPrefixRuleOp[LangSpecParserNodeKind]{
			{
				RightBP:  40,
				NodeKind: NodeParseModifierPredict,
				Rule:     b.predictModifier(),
			},
		},

		PostfixOps: []rule.PrattPostfixOp[LangSpecParserNodeKind]{
			b.g.PostfixOp(TokOptional, 30, NodeParseOptional),
			b.g.PostfixOp(TokStar, 30, NodeParseStar),
			b.g.PostfixOp(TokPlus, 30, NodeParsePlus),
		},

		PostfixRuleOps: []rule.PrattPostfixRuleOp[LangSpecParserNodeKind]{
			b.g.PostfixRuleOp(30, NodeRepetition, b.patternRepetition()),
		},

		InfixOps: []rule.PrattInfixOp[LangSpecParserNodeKind]{
			b.g.InfixOp(TokPipe, 10, 9, NodeParseAlternation),
		},

		ImplicitInfix: &rule.PrattImplicitInfix[LangSpecParserNodeKind]{
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
		b.nestOrEmbedMapping(),
		b.parseGroup(),
		b.usingPattern("PARSE_SEGMENT"),
		b.segmentExplicitTemplateCall(),
		b.segmentIdentOrCall(),
	)
}

func (b *dslGrammarBuilder) segmentExplicitTemplateCall() Rule {
	return b.g.sequence(NodeParseSegment, "TEMPLATE_CALL_SEGMENT").
		expectToken(NodeParseTemplateCallKeyword, TokKWCall).
		expectToken(NodeParseTemplateReference, TokIdentifier).
		rule(b.segmentTemplateCallTail("SEGMENT")).
		build()
}

func (b *dslGrammarBuilder) segmentIdentOrCall() Rule {
	// 1. The Common Prefix (Left-Factored)
	baseChoice := b.g.rb.Rule.Choice(
		"DUMMY_CHOICE_IDENT_MAP_BASE",
		b.g.expectToken(NodeParseSymbolReference, TokIdentifier),
		b.g.expectToken(NodeParseTemplateParameterReference, TokParameter),
	)

	// 2. Optional output mapping (`: Target`). Template invocation uses `call Name(…)` only.
	return b.g.sequence(NodeParseSegment, "IDENT_MAPPING_OR_CALL").
		rule(baseChoice).
		optionalRule(b.segmentMappingTail()).
		build()
}

func (b *dslGrammarBuilder) segmentMappingTail() Rule {
	groupRule := b.g.NestByNode(
		NodeParseGroup,
		TokParenOpen,
		TokParenClose,
		b.parseExpressionReference(),
	)

	targetChoice := b.g.ChoiceByNodeWithSuffix(NodeParseSegment, "MAPPING_TARGET_CHOICE",
		groupRule,
		b.g.expectToken(NodeParseTokenReference, TokIdentifier),
		b.g.expectToken(NodeParseTemplateParameterReference, TokParameter),
	)

	return b.g.sequence(NodeParseSegment, "MAPPING_TAIL").
		expectVirtualInRule(TokColon).
		rule(targetChoice).
		build()
}

func (b *dslGrammarBuilder) segmentTemplateCallTail(suffix string) Rule {
	templateArgRule := b.g.expectOneOf(NodeParseTemplateCallArgument, TokIdentifier, TokParameter, TokPairReference)

	// NestByNodeWithSuffix: NodeParseTemplateCallArgs must not share one memoized nest across
	// SEGMENT / USING_LEX / USING_NEST / USING_PARSE_SEGMENT — otherwise only one variant's
	// LIST_DUMMY_* / LIST_TAIL_* chain is wired and stage-1 reachability falsely warns V_PAR003.
	return b.g.NestByNodeWithSuffix(
		NodeParseTemplateCallArgs,
		suffix,
		TokParenOpen,
		TokParenClose,
		b.g.rb.Rule.TransparentSequence(
			LangSpecGrammarIDFromNodeWithSuffix(NodeParseTemplateCallArgs, "LIST_DUMMY_"+suffix),
			templateArgRule,
			b.g.rb.Rule.TransparentZeroOrMore(
				LangSpecGrammarIDFromNodeWithSuffix(NodeParseTemplateCallArgs, "LIST_TAIL_"+suffix),
				b.g.rb.Rule.TransparentSequence(
					LangSpecGrammarIDFromNodeWithSuffix(NodeParseTemplateCallArgs, "LIST_TAIL_CONTENT_"+suffix),
					b.g.rb.Token.ExpectVirtual("COMMA", lexarch.TokenKind(TokComma)),
					templateArgRule,
				),
			),
		),
	)
}

func (b *dslGrammarBuilder) virtualMapping() Rule {
	return b.g.sequence(NodeParseOpSuppress, "").
		expectVirtualInRule(TokKWVirtual).
		rule(b.g.rb.Rule.Choice(
			"DUMMY_CHOICE_VIRTUAL_MAPPING",
			b.g.expectToken(NodeParseTokenReference, TokIdentifier),
			b.g.expectToken(NodeParseTemplateParameterReference, TokParameter),
		)).
		build()
}

func (b *dslGrammarBuilder) nestOrEmbedMapping() Rule {
	// 1. Extract the shared delimiter resolution logic
	delimitersChoice := b.g.rb.Rule.Choice(
		LangSpecGrammarIDFromNodeWithSuffix(NodeParseOpNest, "CHOICE"),
		b.g.rb.Rule.TransparentSequence(
			LangSpecGrammarIDFromNodeWithSuffix(NodeParseOpNest, "EXPLICIT"),
			b.g.expectToken(NodeParseNestOpenToken, TokIdentifier),
			b.g.expectToken(NodeParseNestCloseToken, TokIdentifier),
		),
		b.usingPattern("NEST"),
		b.g.expectToken(NodeParseNestPairRef, TokPairReference),
		b.g.rb.Rule.TransparentSequence(
			LangSpecGrammarIDFromNodeWithSuffix(NodeParseOpNest, "TPL_PARAM_DELIMS"),
			b.g.expectToken(NodeParseTemplateParameterReference, TokParameter),
			b.g.rb.Rule.Optional(
				b.g.expectToken(NodeParseTemplateParameterReference, TokParameter),
			),
		),
	)

	// 2. Define the standard nest (requires a body, allows sync)
	standardNest := b.g.sequence(NodeParseOpNest, "STANDARD_NEST").
		expectVirtualInRule(TokKWNest).
		rule(delimitersChoice).
		optionalRule(b.syncModifier()).
		rule(b.g.rb.Rule.Choice(
			"DUMMY_CHOICE_NEST",
			b.g.expectToken(NodeParseExpressionReference, TokIdentifier),
			b.g.NestByNode(
				NodeParseNestBody,
				TokBraceOpen,
				TokBraceClose,
				b.g.rb.Rule.Required(
					b.parseExpressionReference(),
					"rule expression needs at least one expression",
				),
			))).
		build()

	// 3. Define the embed nest (no body, no sync)
	embedNest := b.g.sequence(NodeParseEmbedStatement, "EMBED_NEST").
		expectVirtualInRule(TokKWEmbed).
		expectToken(NodeModuleReference, TokIdentifier).
		expectVirtualInRule(TokKWNest).
		rule(delimitersChoice).
		build()

	// 4. Return them as a choice at the PARSE_SEGMENT level
	return b.g.rb.Rule.Choice("NEST_OR_EMBED", embedNest, standardNest)
}

func (b *dslGrammarBuilder) parseGroup() Rule {
	return b.g.NestByNode(
		NodeParseGroup,
		TokParenOpen,
		TokParenClose,
		b.parseExpressionReference(),
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
		expectVirtualInRule(TokColon).
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

func (b *dslGrammarBuilder) parseExpressionReference() Rule {
	return b.g.rb.Rule.Reference(
		syntaxa.GrammarLabel("PARSE EXPRESSION PROXY"),
		VirtualGrammarIDToGrammarID(VirtualParseExpression),
	)
}

func (b *dslGrammarBuilder) declarePatternRef() {
	r := b.g.expectToken(NodePatternRef, TokIdentifier)
	b.g.rb.Rule.Define(r)
}

func (b *dslGrammarBuilder) declareParseExpression() {
	r := b.parseRuleExpr()
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
