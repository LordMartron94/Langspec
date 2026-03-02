package editor

import (
	"autarch/pattern"
	"foundation/domain"
	"langspec/dsl"
	langspeceditor "langspec/editor"
	"langspec/editor/sublime"
)

var runeFactory = pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())

/*
BuildSublimeSyntaxForDSL generates a Sublime Text syntax definition file for the
LangSpec DSL from the given compiler. It builds an editor IR with token overrides
(line comments with capture, regex embed) and a node scope override for DSLName
(entity.name.language); delimited block comments are generated automatically from the ruleset.
Then writes the result to syntaxFile.

Use this from tools or tests that need .lspec syntax highlighting; the core
langspec/dsl compiler does not depend on editor or sublime.
*/
func BuildSublimeSyntaxForDSL(compiler *dsl.LangSpecCompiler, syntaxFile string) error {
	scopeMap := dsl.LangSpecCompilerScopeMap(compiler)
	scopeResolver := func(t dsl.LangSpecLexerTokenType) string { return scopeMap[t] }

	editorIRConfig := langspeceditor.PushDownAutomatonIRConfigurationCreate[dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole](
		scopeResolver,
		dsl.LangSpecLexerTokenType.String,
		".lspec",
	)

	editorIRConfig.AddPrototypeTokenRoles(
		dsl.LANG_SPEC_WHITESPACE_ROLE,
		dsl.LANG_SPEC_COMMENT_ROLE,
	)

	editorIRConfig.AddOverride(dsl.TokLineComment, func(ctx *langspeceditor.TokenOverrideContext) (langspeceditor.StateRule, []langspeceditor.State) {
		slashes := pattern.LiteralString(runeFactory, "//").Capture()
		notTerminator := runeFactory.NegatedClass(
			runeFactory.Range('\n', '\n'),
			runeFactory.Range('\r', '\r'),
		).Star().Capture()
		regex, _ := slashes.Then(notTerminator).ToRegEx()
		return langspeceditor.TokenOverrideMatchWithCapture(ctx,
			regex,
			"comment.line.double-slash",
			"punctuation.definition.comment",
		)
	})

	backtickRegex, _ := pattern.LiteralString(runeFactory, "`").ToRegEx()
	editorIRConfig.AddOverride(dsl.TokRegexLiteral, func(ctx *langspeceditor.TokenOverrideContext) (langspeceditor.StateRule, []langspeceditor.State) {
		return langspeceditor.TokenOverrideEmbed(ctx,
			backtickRegex,
			"punctuation.definition.string.begin",
			"scope:source.regexp",
			"meta.embedded.regexp",
			"`",
			map[int]string{0: "punctuation.definition.string.end"},
		)
	})

	editorIRConfig.AddNodeScopeOverride(dsl.LangSpecGrammarIDFromNode(dsl.NodeDSLName), "entity.name.language")
	editorIRConfig.AddNodeScopeOverride(dsl.LangSpecGrammarIDFromNode(dsl.NodePatternAlternation), "keyword.operator.alternation")

	editorIRConfig.AddNodeScopeOverride(dsl.LangSpecGrammarIDFromNode(dsl.NodePatternDefName), "entity.name.variable.constant")
	editorIRConfig.AddNodeScopeOverride(dsl.LangSpecGrammarIDFromNode(dsl.NodePatternVarRefToken), "punctuation.reference.variable")
	editorIRConfig.AddNodeScopeOverride(dsl.LangSpecGrammarIDFromNode(dsl.NodePatternVarRefTarget), "variable.constant.reference.target")
	editorIRConfig.AddNodeOverride(dsl.LangSpecGrammarIDFromNode(dsl.NodePatternVarRef), langspeceditor.OverrideConfig{MetaScope: "meta.variable.reference"})

	editorIRConfig.AddNodeScopeOverride(dsl.LangSpecGrammarIDFromNode(dsl.NodeLexRuleTokenName), "entity.name.token")
	editorIRConfig.AddNodeScopeOverride(dsl.LangSpecGrammarIDFromNode(dsl.NodeLexRuleRole), "entity.name.token-role")

	editorIRConfig.AddNodeScopeOverride(dsl.LangSpecGrammarIDFromNode(dsl.NodeMetaKey), "entity.other.attribute-name.meta")
	editorIRConfig.AddNodeScopeOverride(dsl.LangSpecGrammarIDFromNode(dsl.NodeMetaValue), "meta.annotation.value", "string.quoted.double")

	lexingRuleSet := dsl.LangSpecCompilerLexingRuleSet(compiler)
	grammarPackage := dsl.LangSpecCompilerGrammarPackage(compiler)

	editorIR := langspeceditor.PushDownAutomatonIRCreate(
		editorIRConfig,
		lexingRuleSet,
		*grammarPackage,
	)

	return sublime.SublimeTextGenerateSyntaxFile(
		editorIR,
		syntaxFile,
		[]string{".lspec"},
	)
}
