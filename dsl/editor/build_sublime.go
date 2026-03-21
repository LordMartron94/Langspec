package editor

import (
	"autarch/pattern"
	"fmt"
	"foundation/bytes"
	"foundation/domain"
	"foundation/hash"
	"langspec/dsl"
	dslspec "langspec/dsl/spec"
	langspeceditor "langspec/editor"
	"langspec/toolchain"
	"lexarch"
)

var hasher = hash.XXH3HasherCreateWithSeed(6789)

var runeFactory = pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())

type EditorCtx = langspeceditor.EditorCtx[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind]
type EditorOverride = langspeceditor.EditorOverride[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind, toolchain.SublimeContext]

func BuildSublimeSyntaxForDSL(compiler *dsl.LangSpecCompiler, syntaxFile string) error {
	manifest := LangSpecEditorManifest
	manifest.BaseTokenScopes = dsl.LangSpecCompilerScopeMap(compiler)

	ctxProducer := toolchain.BuildContextProducerFromManifest[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState](manifest)

	ruleset := dsl.LangSpecCompilerLexingRuleSet(compiler)

	irConfig := langspeceditor.EditorIRConfigurationCreate(
		hasher,
		func(token dsl.LangSpecLexerTokenType) uint64 {
			return hash.XXH3HasherHash64(hasher, bytes.StringSliceToBytes([]string{token.String()}, 0x00))
		},
		ctxProducer,
		BuildEditorOverrideProducer(ruleset, ctxProducer),
		func(left, right toolchain.SublimeContext) bool {
			return left == right
		},
	)

	runnerCfg := &toolchain.SublimeRunnerConfig[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecLexerState, dsl.LangSpecParserNodeKind]{
		LexerRuleset:   ruleset,
		GrammarPackage: dsl.LangSpecCompilerGrammarPackage(compiler),
		IRConfig:       irConfig,
		FileExtensions: []string{".lspec"},
		BaseScope:      "source.lspec",
		OutputPath:     syntaxFile,
		ScopeSuffix:    ".lspec",
	}

	return toolchain.RunSublimeGenerator(runnerCfg)
}

func BuildEditorOverrideProducer(
	ruleset *lexarch.LexingRuleset[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole],
	ctxProducer func(ctx *EditorCtx) toolchain.SublimeContext,
) func(editorCtx *EditorCtx) []*EditorOverride {
	return func(editorCtx *EditorCtx) []*EditorOverride {
		if editorCtx.Token == nil {
			return nil
		}

		// Route to the dynamic identifier ref override
		if *editorCtx.Token == dslspec.TokIdentifier && editorCtx.NodeKind != nil && *editorCtx.NodeKind == dslspec.NodeParseSymbolReference {
			return identifierRefOverrides(ruleset, ctxProducer)
		}

		switch *editorCtx.Token {
		case dslspec.TokLineComment:
			return []*EditorOverride{lineCommentOverride()}
		case dslspec.TokBlockComment:
			return []*EditorOverride{blockCommentOverride()}
		case dslspec.TokRegexLiteral:
			return []*EditorOverride{regExOverride()}
		default:
			return nil
		}
	}
}

func identifierRefOverrides(
	ruleset *lexarch.LexingRuleset[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole],
	ctxProducer func(ctx *EditorCtx) toolchain.SublimeContext,
) []*EditorOverride {
	identRegex := getTokenRegex(ruleset, dslspec.TokIdentifier)
	assignRegex := getTokenRegex(ruleset, dslspec.TokAssignment)

	mappingRegex := fmt.Sprintf(`%s(?=\s*%s)`, identRegex, assignRegex)
	refRegex := fmt.Sprintf(`%s(?!\s*%s)`, identRegex, assignRegex)

	// 2. Resolve exact scopes dynamically from the Context Producer
	mappingNode := dslspec.NodeParseNodeName
	mappingCtx := ctxProducer(&EditorCtx{NodeKind: &mappingNode})

	refNode := dslspec.NodeParseExpressionReference
	refCtx := ctxProducer(&EditorCtx{NodeKind: &refNode})

	return []*EditorOverride{
		{
			PatternRegex: &mappingRegex,
			MatchContext: &mappingCtx,
		},
		{
			PatternRegex: &refRegex,
			MatchContext: &refCtx,
		},
	}
}

// Helper to extract the regex string natively from your lexer rules
func getTokenRegex(
	ruleset *lexarch.LexingRuleset[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole],
	token dsl.LangSpecLexerTokenType,
) string {
	for _, rule := range ruleset.GetRules() {
		if rule.Token == token {
			regex, err := rule.Pattern.ToRegEx()
			if err != nil {
				panic(fmt.Sprintf("Failed to convert token %v to regex: %v", token, err))
			}
			return regex
		}
	}
	panic(fmt.Sprintf("Token %v not found in lexing ruleset", token))
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

var LangSpecEditorManifest = toolchain.SemanticManifest[dsl.LangSpecLexerTokenType, dsl.LangSpecParserNodeKind]{
	InvalidScope: "invalid.illegal.unexpected-token",
	NodeBindings: map[dsl.LangSpecParserNodeKind]toolchain.NodeBinding[dsl.LangSpecLexerTokenType]{
		dslspec.NodeDSLName:            {Scopes: []string{"entity.name.language"}},
		dslspec.NodePatternAlternation: {Scopes: []string{"keyword.operator.alternation"}},
		dslspec.NodePatternDefName:     {Scopes: []string{"entity.name.pattern.constant"}},
		dslspec.NodePatternRef:         {Scopes: []string{"constant.language.pattern-reference"}},
		dslspec.NodeLexRuleTokenName:   {Scopes: []string{"entity.name.token"}},
		dslspec.NodeLexRuleRole:        {Scopes: []string{"entity.name.token-role"}},
		dslspec.NodeMetaKey:            {Scopes: []string{"entity.other.attribute-name.meta"}},
		dslspec.NodeMetaValue: {
			Scopes: []string{"meta.annotation.value"},
			TokenScopes: map[dsl.LangSpecLexerTokenType][]string{
				dslspec.TokStringLiteral: {"meta.annotation.value", "string.quoted.double"},
				dslspec.TokKWTrue:        {"meta.annotation.value", "constant.language.bool"},
				dslspec.TokKWFalse:       {"meta.annotation.value", "constant.language.bool"},
			},
		},
		dslspec.NodePragmaConfiguration:   {MetaScope: "meta.pragma.configuration"},
		dslspec.NodePragmaBlockKeySegment: {Scopes: []string{"entity.name.namespace"}},
		dslspec.NodePragmaKey:             {Scopes: []string{"entity.other.attribute-name"}},
		dslspec.NodePragmaValue: {
			Scopes: []string{"entity.other.attribute-value"},
			TokenScopes: map[dsl.LangSpecLexerTokenType][]string{
				dslspec.TokStringLiteral: {"entity.other.attribute-value", "string.quoted.double"},
				dslspec.TokKWTrue:        {"entity.other.attribute-value", "constant.language.bool"},
				dslspec.TokKWFalse:       {"entity.other.attribute-value", "constant.language.bool"},
			},
		},
		dslspec.NodeParseRuleName:            {Scopes: []string{"entity.name.function.parser-expression"}},
		dslspec.NodeParseNodeName:            {Scopes: []string{"entity.name.type.parser-node"}},
		dslspec.NodeParseSymbolReference:     {Scopes: []string{"constant.language.symbol-reference"}},
		dslspec.NodeParseExpressionReference: {Scopes: []string{"entity.name.function.expression-reference"}},
		dslspec.NodeParseTokenReference:      {Scopes: []string{"constant.language.token-reference"}},
		dslspec.NodeParseNestOpenToken:       {Scopes: []string{"constant.language.token-reference"}},
		dslspec.NodeParseNestCloseToken:      {Scopes: []string{"constant.language.token-reference"}},
		dslspec.NodePrattExprName:            {Scopes: []string{"entity.name.function.parser-rule"}},
		dslspec.NodeParseIgnoreRole:          {Scopes: []string{"constant.language.token-role-reference"}},
		dslspec.NodePredictToken:             {Scopes: []string{"constant.language.token-reference"}},
		dslspec.NodeSyncToken:                {Scopes: []string{"constant.language.token-reference"}},
	},
}
