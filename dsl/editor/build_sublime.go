package editor

import (
	"autarch/pattern"
	"foundation/domain"
	"langspec/dsl"
	langspeceditor "langspec/editor"
	"langspec/editor/sublime"
)

var runeFactory = pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())

type EditorCtx = langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind]
type EditorOverride = langspeceditor.EditorOverride[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind, SublimeContext]

type SublimeContext struct {
	Scope     string
	MetaScope string
}

// ------------------------------------------------------------- ORCHESTRATOR

func BuildSublimeSyntaxForDSL(compiler *dsl.LangSpecCompiler, syntaxFile string) error {
	scopeMap := dsl.LangSpecCompilerScopeMap(compiler)

	config := langspeceditor.EditorIRConfigurationCreate(
		dsl.LangSpecLexerTokenType.String,
		buildContextProducer(scopeMap),
		buildEditorOverrideProducer(),
	)

	editorIR := langspeceditor.EditorIRCreate(
		dsl.LangSpecCompilerLexingRuleSet(compiler),
		*dsl.LangSpecCompilerGrammarPackage(compiler),
		config,
	)

	err := sublime.GenerateSyntaxFile(
		editorIR,
		[]string{".lspec"},
		"source.lspec",
		syntaxFile,
		sublime.ExtractionConfig[SublimeContext]{
			ExtractScope: func(sc SublimeContext) string {
				return sc.Scope
			},
			ExtractMetaScope: func(sc SublimeContext) string {
				return sc.MetaScope
			},
		},
	)

	return err
}

// ------------------------------------------------------------- CONTEXT PIPELINE

func buildContextProducer(
	scopeMap map[dsl.LangSpecLexerTokenType]string,
) func(*EditorCtx) SublimeContext {

	return func(ctx *langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind]) SublimeContext {
		// 1. Resolve base lexical scope
		baseScope := resolveBaseScope(ctx, scopeMap)

		// 2. Apply Node/AST Overrides (Dead code path until IR processes Parser Nodes)
		baseScope = applyNodeOverrides(ctx, baseScope)

		return SublimeContext{
			Scope: baseScope,
		}
	}
}

func resolveBaseScope(
	ctx *EditorCtx,
	scopeMap map[dsl.LangSpecLexerTokenType]string,
) string {
	if ctx.Token == nil {
		return ""
	}
	return scopeMap[*ctx.Token]
}

func applyNodeOverrides(
	ctx *EditorCtx,
	currentScope string,
) string {
	return currentScope
}

func buildEditorOverrideProducer() func(editorCtx *EditorCtx) (override *EditorOverride, hasOverride bool) {
	return func(editorCtx *EditorCtx) (override *EditorOverride, hasOverride bool) {
		if editorCtx.Token == nil {
			return nil, false
		}

		switch *editorCtx.Token {
		case dsl.TokLineComment:
			return lineCommentOverride(), true
		case dsl.TokBlockComment:
			return blockCommentOverride(), true
		case dsl.TokRegexLiteral:
			return regExOverride(), true
		default:
			return nil, false
		}
	}
}

func lineCommentOverride() *EditorOverride {
	// Group 1: The slashes
	slashes := pattern.LiteralString(runeFactory, "//").Capture()

	// Group 2: The actual comment text (not terminator)
	notTerminator := runeFactory.NegatedClass(
		runeFactory.Range('\n', '\n'),
		runeFactory.Range('\r', '\r'),
	).Star().Capture()

	newPattern := slashes.Then(notTerminator)

	matchCtx := SublimeContext{Scope: "comment.line.double-slash"}

	return &EditorOverride{
		Pattern:      &newPattern,
		MatchContext: &matchCtx,
		Captures: map[int]SublimeContext{
			1: {Scope: "punctuation.definition.comment"},
		},
	}
}

func blockCommentOverride() *EditorOverride {
	openPattern := pattern.LiteralString(runeFactory, "/*")
	closePattern := pattern.LiteralString(runeFactory, "*/")

	return &EditorOverride{
		Pattern:      &openPattern,
		MatchContext: &SublimeContext{Scope: "punctuation.definition.comment.begin"},
		DelimitedPayload: &langspeceditor.DelimitedPayload[rune, SublimeContext]{
			StateLabel:   "block_comment_inner",
			BodyContext:  SublimeContext{MetaScope: "comment.block"},
			ClosePattern: closePattern,
			CloseContext: SublimeContext{Scope: "punctuation.definition.comment.end"},
		},
	}
}

func regExOverride() *EditorOverride {
	backtick := pattern.LiteralString(runeFactory, "`")

	return &EditorOverride{
		Pattern:      &backtick,
		MatchContext: &SublimeContext{Scope: "punctuation.definition.string.begin"},
		ForeignPayload: &langspeceditor.ForeignMachinePayload[rune, SublimeContext]{
			MachineID:      "scope:source.regexp",
			MachineContext: SublimeContext{Scope: "meta.embedded.regexp"},
			EscapePattern:  backtick,
			EscapeCaptures: map[int]SublimeContext{
				0: {Scope: "punctuation.definition.string.end"},
			},
		},
	}
}

// ------------------------------------------------------------- NODE BINDING (PRESERVED)

type NodeBinding struct {
	Scopes      []string
	MetaScope   string
	TokenScopes map[dsl.LangSpecLexerTokenType][]string
}

var langSpecEditorManifest = map[dsl.LangSpecParserNodeKind]NodeBinding{
	dsl.NodeDSLName:            {Scopes: []string{"entity.name.language"}},
	dsl.NodePatternAlternation: {Scopes: []string{"keyword.operator.alternation"}},
	dsl.NodePatternDefName:     {Scopes: []string{"entity.name.pattern.constant"}},
	dsl.NodePatternRef:         {Scopes: []string{"constant.language.pattern-reference"}},
	dsl.NodeLexRuleTokenName:   {Scopes: []string{"entity.name.token"}},
	dsl.NodeLexRuleRole:        {Scopes: []string{"entity.name.token-role"}},
	dsl.NodeMetaKey:            {Scopes: []string{"entity.other.attribute-name.meta"}},
	dsl.NodeMetaValue: {
		Scopes: []string{"meta.annotation.value"},
		TokenScopes: map[dsl.LangSpecLexerTokenType][]string{
			dsl.TokStringLiteral: {"meta.annotation.value", "string.quoted.double"},
			dsl.TokKWTrue:        {"meta.annotation.value", "constant.language.bool"},
			dsl.TokKWFalse:       {"meta.annotation.value", "constant.language.bool"},
		},
	},
	dsl.NodePragmaConfiguration:   {MetaScope: "meta.pragma.configuration"},
	dsl.NodePragmaBlockKeySegment: {Scopes: []string{"entity.name.namespace"}},
	dsl.NodePragmaKey:             {Scopes: []string{"entity.other.attribute-name"}},
	dsl.NodePragmaValue: {
		Scopes: []string{"entity.other.attribute-value"},
		TokenScopes: map[dsl.LangSpecLexerTokenType][]string{
			dsl.TokStringLiteral: {"entity.other.attribute-value", "string.quoted.double"},
			dsl.TokKWTrue:        {"entity.other.attribute-value", "constant.language.bool"},
			dsl.TokKWFalse:       {"entity.other.attribute-value", "constant.language.bool"},
		},
	},
	dsl.NodeParseRuleName:            {Scopes: []string{"entity.name.function.parser-expression"}},
	dsl.NodeParseNodeName:            {Scopes: []string{"entity.name.type.parser-node"}},
	dsl.NodeParseSymbolReference:     {Scopes: []string{"constant.language.symbol-reference"}},
	dsl.NodeParseExpressionReference: {Scopes: []string{"entity.name.function.expression-reference"}},
	dsl.NodeParseTokenReference:      {Scopes: []string{"constant.language.token-reference"}},
	dsl.NodeParseNestOpenToken:       {Scopes: []string{"constant.language.token-reference"}},
	dsl.NodeParseNestCloseToken:      {Scopes: []string{"constant.language.token-reference"}},
	dsl.NodePrattExprName:            {Scopes: []string{"entity.name.function.parser-rule"}},
	dsl.NodeParseIgnoreRole:          {Scopes: []string{"constant.language.token-role-reference"}},
	dsl.NodePredictToken:             {Scopes: []string{"constant.language.token-reference"}},
	dsl.NodeSyncToken:                {Scopes: []string{"constant.language.token-reference"}},
}
