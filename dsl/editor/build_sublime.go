package editor

import (
	"autarch/pattern"
	"foundation/bytes"
	"foundation/domain"
	"foundation/hash"
	"langspec/dsl"
	"langspec/dsl/generator"
	langspeceditor "langspec/editor"
	"langspec/toolchain"
)

var hasher = hash.XXH3HasherCreateWithSeed(6789)

var runeFactory = pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())

type EditorCtx = langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind]
type EditorOverride = langspeceditor.EditorOverride[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind, toolchain.SublimeContext]

func BuildSublimeSyntaxForDSL(compiler *dsl.LangSpecCompiler, syntaxFile string) error {
	scopeMap := dsl.LangSpecCompilerScopeMap(compiler)

	ctxCfg := toolchain.ContextProducerConfig[dsl.LangSpecLexerTokenType, dsl.LangSpecParserNodeKind]{
		InvalidScope: "invalid.illegal.unexpected-token",
		GetBaseScope: func(token dsl.LangSpecLexerTokenType) string {
			return scopeMap[token]
		},
		GetNodeScope: func(nodeKind dsl.LangSpecParserNodeKind, token *dsl.LangSpecLexerTokenType) string {
			binding, ok := langSpecEditorManifest[nodeKind]
			if !ok {
				return ""
			}
			if token != nil && len(binding.TokenScopes) > 0 {
				if ts, ok := binding.TokenScopes[*token]; ok && len(ts) > 0 {
					return ts[0]
				}
			}
			if len(binding.Scopes) > 0 {
				return binding.Scopes[0]
			}
			return ""
		},
	}

	irConfig := langspeceditor.EditorIRConfigurationCreate(
		hasher,
		func(token dsl.LangSpecLexerTokenType) uint64 {
			return hash.XXH3HasherHash64(hasher, bytes.StringSliceToBytes([]string{token.String()}, 0x00))
		},
		toolchain.BuildContextProducer[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState](ctxCfg),
		BuildEditorOverrideProducer(),
		func(left, right toolchain.SublimeContext) bool {
			return left == right
		},
	)

	runnerCfg := &toolchain.SublimeRunnerConfig[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind]{
		LexerRuleset:   dsl.LangSpecCompilerLexingRuleSet(compiler),
		GrammarPackage: dsl.LangSpecCompilerGrammarPackage(compiler),
		IRConfig:       irConfig,
		FileExtensions: []string{".lspec"},
		BaseScope:      "source.lspec",
		OutputPath:     syntaxFile,
		ScopeSuffix:    ".lspec",
	}

	return toolchain.RunSublimeGenerator(runnerCfg)
}

func BuildEditorOverrideProducer() func(editorCtx *EditorCtx) (override *EditorOverride, hasOverride bool) {
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
	matchCtx := toolchain.SublimeContext{Scope: "comment.line.double-slash"}

	return &EditorOverride{
		Pattern:      &newPattern,
		MatchContext: &matchCtx,
		Captures: map[int]toolchain.SublimeContext{
			1: {Scope: "punctuation.definition.comment"},
		},
	}
}

func blockCommentOverride() *EditorOverride {
	openPattern := pattern.LiteralString(runeFactory, "/*")
	closePattern := pattern.LiteralString(runeFactory, "*/")

	return &EditorOverride{
		Pattern:      &openPattern,
		MatchContext: &toolchain.SublimeContext{Scope: "punctuation.definition.comment.begin"},
		DelimitedPayload: &langspeceditor.DelimitedPayload[rune, toolchain.SublimeContext]{
			StateLabel:   "block_comment_inner",
			BodyContext:  toolchain.SublimeContext{MetaScope: "comment.block"},
			ClosePattern: closePattern,
			CloseContext: toolchain.SublimeContext{Scope: "punctuation.definition.comment.end"},
		},
	}
}

func regExOverride() *EditorOverride {
	backtick := pattern.LiteralString(runeFactory, "`")

	return &EditorOverride{
		Pattern:      &backtick,
		MatchContext: &toolchain.SublimeContext{Scope: "punctuation.definition.string.begin"},
		ForeignPayload: &langspeceditor.ForeignMachinePayload[rune, toolchain.SublimeContext]{
			MachineID:      "scope:source.regexp",
			MachineContext: toolchain.SublimeContext{MetaScope: "meta.embedded.regexp"},
			EscapePattern:  backtick,
			EscapeCaptures: map[int]toolchain.SublimeContext{
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

func GetGeneratorBindings() map[dsl.LangSpecParserNodeKind]generator.NodeBinding[dsl.LangSpecLexerTokenType, dsl.LangSpecParserNodeKind] {
	out := make(map[dsl.LangSpecParserNodeKind]generator.NodeBinding[dsl.LangSpecLexerTokenType, dsl.LangSpecParserNodeKind])

	for kind, b := range langSpecEditorManifest {
		out[kind] = generator.NodeBinding[dsl.LangSpecLexerTokenType, dsl.LangSpecParserNodeKind]{
			Scopes:      b.Scopes,
			TokenScopes: b.TokenScopes,
		}
	}
	return out
}
