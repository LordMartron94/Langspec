package editor

import (
	"autarch/pattern"
	"foundation/domain"
	"langspec/dsl"
	langspeceditor "langspec/editor"
	"langspec/editor/sublime"
	"memarch"
	"strings"
	"syntaxa"
)

var runeFactory = pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())

type EditorCtx = langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind]
type EditorOverride = langspeceditor.EditorOverride[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind, SublimeContext]

type SublimeContext struct {
	Scope     string
	MetaScope string
}

func BuildSublimeSyntaxForDSL(compiler *dsl.LangSpecCompiler, syntaxFile string, pdaAllocationFn memarch.AllocationFn) error {
	scopeMap := dsl.LangSpecCompilerScopeMap(compiler)

	config := langspeceditor.EditorIRConfigurationCreate(
		dsl.LangSpecLexerTokenType.String,
		buildContextProducer(scopeMap),
		buildEditorOverrideProducer(),
		func(left, right SublimeContext) bool {
			return left.Scope == right.Scope && left.MetaScope == right.MetaScope
		},
		pdaAllocationFn,
	).WithNestContextProducer(buildNestContextProducer())

	editorIR, err := langspeceditor.EditorIRCreate(
		dsl.LangSpecCompilerLexingRuleSet(compiler),
		dsl.LangSpecCompilerGrammarPackage(compiler),
		config,
	)

	if err != nil {
		return err
	}

	return sublime.GenerateSyntaxFile(
		editorIR,
		[]string{".lspec"},
		"source.lspec",
		syntaxFile,
		buildExtractionConfig(".lspec"),
	)
}

func buildExtractionConfig(suffix string) sublime.ExtractionConfig[SublimeContext] {
	return sublime.ExtractionConfig[SublimeContext]{
		ExtractScope: func(ctx SublimeContext) string {
			return applyScopeSuffix(ctx.Scope, suffix)
		},
		ExtractMetaScope: func(ctx SublimeContext) string {
			return applyScopeSuffix(ctx.MetaScope, suffix)
		},
	}
}

func applyScopeSuffix(scope, suffix string) string {
	if scope == "" {
		return ""
	}
	return scope + suffix
}

func buildNestContextProducer() func(syntaxa.GrammarLabel) SublimeContext {
	return func(label syntaxa.GrammarLabel) SublimeContext {
		return SublimeContext{MetaScope: nestLabelToMetaScope(string(label))}
	}
}

func nestLabelToMetaScope(label string) string {
	if label == "" {
		return ""
	}
	lower := strings.ToLower(label)
	lower = strings.TrimSuffix(lower, "_nest")
	lower = strings.TrimSuffix(lower, " nest")
	lower = strings.ReplaceAll(lower, " ", "-")
	lower = strings.ReplaceAll(lower, "_", "-")
	return "meta." + lower + ".body"
}

func buildContextProducer(
	scopeMap map[dsl.LangSpecLexerTokenType]string,
) func(*EditorCtx) SublimeContext {
	return func(ctx *langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind]) SublimeContext {
		baseScope := resolveBaseScope(ctx, scopeMap)
		baseScope = applyNodeOverrides(ctx, baseScope)
		return SublimeContext{Scope: baseScope}
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
	if ctx.NodeKind == nil || ctx.Token == nil {
		return currentScope
	}

	binding, ok := langSpecEditorManifest[*ctx.NodeKind]
	if !ok {
		return currentScope
	}

	if len(binding.TokenScopes) > 0 {
		if tokenScopes, hasTokenOverride := binding.TokenScopes[*ctx.Token]; hasTokenOverride && len(tokenScopes) > 0 {
			return tokenScopes[0]
		}
	}

	if len(binding.Scopes) > 0 {
		return binding.Scopes[0]
	}

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
	slashes := pattern.LiteralString(runeFactory, "//").Capture()
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
			MachineContext: SublimeContext{MetaScope: "meta.embedded.regexp"},
			EscapePattern:  backtick,
			EscapeCaptures: map[int]SublimeContext{
				0: {Scope: "punctuation.definition.string.end"},
			},
		},
	}
}

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
