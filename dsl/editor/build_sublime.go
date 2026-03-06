package editor

import (
	"autarch/pattern"
	"foundation/domain"
	"langspec/dsl"
	langspeceditor "langspec/editor"
	"langspec/editor/sublime"
)

// ------------------------------------------------------------- ORCHESTRATOR

func BuildSublimeSyntaxForDSL(compiler *dsl.LangSpecCompiler, syntaxFile string) error {
	scopeMap := dsl.LangSpecCompilerScopeMap(compiler)

	editorIRConfig := langspeceditor.PushDownAutomatonIRConfigurationCreate[dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole](
		func(t dsl.LangSpecLexerTokenType) string { return scopeMap[t] },
		dsl.LangSpecLexerTokenType.String,
		".lspec",
	)

	// 1. Core Configuration
	editorIRConfig.AddPrototypeTokenRoles(dsl.LANG_SPEC_WHITESPACE_ROLE, dsl.LANG_SPEC_COMMENT_ROLE)

	// 2. Custom Token Logic
	editorIRConfig.AddOverride(dsl.TokLineComment, buildLineCommentOverride)
	editorIRConfig.AddOverride(dsl.TokRegexLiteral, buildRegexLiteralOverride)

	// 3. Apply the Declarative Manifest
	for kind, binding := range langSpecEditorManifest {
		grammarID := dsl.LangSpecGrammarIDFromNode(kind)

		editorIRConfig.AddNodeOverride(grammarID, langspeceditor.OverrideConfig[dsl.LangSpecLexerTokenType]{
			Scopes:      binding.Scopes,
			MetaScope:   binding.MetaScope,
			TokenScopes: binding.TokenScopes,
		})
	}

	// 4. Engine Execution
	editorIR := langspeceditor.PushDownAutomatonIRCreate(
		editorIRConfig,
		dsl.LangSpecCompilerLexingRuleSet(compiler),
		*dsl.LangSpecCompilerGrammarPackage(compiler),
	)

	return sublime.SublimeTextGenerateSyntaxFile(editorIR, syntaxFile, []string{".lspec"}, false)
}

// ------------------------------------------------------------- NODE BINDING

type NodeBinding struct {
	Scopes      []string
	MetaScope   string
	TokenScopes map[dsl.LangSpecLexerTokenType][]string
}

var langSpecEditorManifest = map[dsl.LangSpecParserNodeKind]NodeBinding{
	dsl.NodeDSLName:            {Scopes: []string{"entity.name.language"}},
	dsl.NodePatternAlternation: {Scopes: []string{"keyword.operator.alternation"}},
	dsl.NodePatternDefName:     {Scopes: []string{"entity.name.variable.constant"}},
	dsl.NodeVarRefToken:        {Scopes: []string{"punctuation.reference.variable"}},
	dsl.NodeVarRefTarget:       {Scopes: []string{"variable.constant.reference.target"}},
	dsl.NodeVarRef:             {MetaScope: "meta.variable.reference"},
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
	dsl.NodeParseRuleName: {
		Scopes: []string{"entity.name.function.parser-rule"},
	},
	dsl.NodeParseNodeName: {
		Scopes: []string{"entity.name.type.parser-node"},
	},
	dsl.NodeParseTokenReference: {
		Scopes: []string{"constant.language.token-reference"},
	},
	dsl.NodeParseRuleReference: {
		Scopes: []string{"entity.name.function.rule-reference"},
	},
	dsl.NodeParseNestOpenToken: {
		Scopes: []string{"constant.language.token-reference"},
	},
	dsl.NodeParseNestCloseToken: {
		Scopes: []string{"constant.language.token-reference"},
	},
}

// ------------------------------------------------------------- PATTERN BUILDING

var runeFactory = pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())

func buildLineCommentOverride(ctx *langspeceditor.TokenOverrideContext) (langspeceditor.StateRule, []langspeceditor.State) {
	slashes := pattern.LiteralString(runeFactory, "//").Capture()
	notTerminator := runeFactory.NegatedClass(
		runeFactory.Range('\n', '\n'),
		runeFactory.Range('\r', '\r'),
	).Star().Capture()
	regex, _ := slashes.Then(notTerminator).ToRegEx()

	return langspeceditor.TokenOverrideMatchWithCapture(ctx, regex, "comment.line.double-slash", "punctuation.definition.comment")
}

func buildRegexLiteralOverride(ctx *langspeceditor.TokenOverrideContext) (langspeceditor.StateRule, []langspeceditor.State) {
	backtickRegex, _ := pattern.LiteralString(runeFactory, "`").ToRegEx()
	return langspeceditor.TokenOverrideEmbed(ctx,
		backtickRegex,
		"punctuation.definition.string.begin",
		"scope:source.regexp",
		"meta.embedded.regexp",
		"`",
		map[int]string{0: "punctuation.definition.string.end"},
	)
}
