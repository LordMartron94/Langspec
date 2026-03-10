package editor

import (
	"langspec/dsl"
	langspeceditor "langspec/editor"
	"langspec/editor/sublime"
)

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
	)

	editorIR := langspeceditor.EditorIRCreate(
		dsl.LangSpecCompilerLexingRuleSet(compiler),
		*dsl.LangSpecCompilerGrammarPackage(compiler),
		config,
	)

	err := sublime.GenerateSyntaxFile(
		editorIR,
		[]string{".lspec"},
		func(ctx SublimeContext) string { return ctx.Scope },
		"source.lspec",
		syntaxFile,
	)

	return err
}

// ------------------------------------------------------------- CONTEXT PIPELINE

func buildContextProducer(
	scopeMap map[dsl.LangSpecLexerTokenType]string,
) func(*langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind]) SublimeContext {

	return func(ctx *langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind]) SublimeContext {

		// 1. Resolve base lexical scope
		baseScope := resolveBaseScope(ctx, scopeMap)

		// 2. Apply Custom Token Overrides (Regex Literal, Line Comments)
		// Note: These currently just return the base scope until the IR
		// supports mapping transitions to complex embeds/captures.
		baseScope = applyTokenOverrides(ctx, baseScope)

		// 3. Apply Node/AST Overrides (Dead code path until IR processes Parser Nodes)
		baseScope = applyNodeOverrides(ctx, baseScope)

		return SublimeContext{
			Scope: baseScope,
		}
	}
}

func resolveBaseScope(
	ctx *langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind],
	scopeMap map[dsl.LangSpecLexerTokenType]string,
) string {
	if ctx.Token == nil {
		return ""
	}
	return scopeMap[*ctx.Token]
}

func applyTokenOverrides(
	ctx *langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind],
	currentScope string,
) string {
	if ctx.Token == nil {
		return currentScope
	}

	switch *ctx.Token {
	case dsl.TokLineComment:
		return "comment.line.double-slash"
	case dsl.TokRegexLiteral:
		return "string.regexp.literal"
	default:
		return currentScope
	}
}

func applyNodeOverrides(
	ctx *langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind],
	currentScope string,
) string {
	return currentScope
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

// ------------------------------------------------------------- PATTERN BUILDING

// var runeFactory = pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())

// func buildLineCommentOverride(ctx *langspeceditor.TokenOverrideContext) (langspeceditor.StateRule, []langspeceditor.State) {
// 	slashes := pattern.LiteralString(runeFactory, "//").Capture()
// 	notTerminator := runeFactory.NegatedClass(
// 		runeFactory.Range('\n', '\n'),
// 		runeFactory.Range('\r', '\r'),
// 	).Star().Capture()
// 	regex, _ := slashes.Then(notTerminator).ToRegEx()

// 	return langspeceditor.TokenOverrideMatchWithCapture(ctx, regex, "comment.line.double-slash", "punctuation.definition.comment")
// }

// func buildRegexLiteralOverride(ctx *langspeceditor.TokenOverrideContext) (langspeceditor.StateRule, []langspeceditor.State) {
// 	backtickRegex, _ := pattern.LiteralString(runeFactory, "`").ToRegEx()
// 	return langspeceditor.TokenOverrideEmbed(ctx,
// 		backtickRegex,
// 		"punctuation.definition.string.begin",
// 		"scope:source.regexp",
// 		"meta.embedded.regexp",
// 		"`",
// 		map[int]string{0: "punctuation.definition.string.end"},
// 	)
// }
