package editor

import (
	"autarch/pattern"
	"foundation/domain"
	"langspec/dsl"
	langspeceditor "langspec/editor"
	"langspec/editor/sublime"
	"syntaxa"
)

var runeFactory = pattern.RegulaASTFactoryCreate(domain.DiscreteDomainRuneCreate())

/*
BuildSublimeSyntaxForDSL generates a Sublime Text syntax definition file for the
LangSpec DSL from the given compiler. It builds an editor IR with token overrides
(delimited block comments, line comments with capture) and a nest override for the
HEADER production, then writes the result to syntaxFile.

Use this from tools or tests that need .lspec syntax highlighting; the core
langspec/dsl compiler does not depend on editor or sublime.
*/
func BuildSublimeSyntaxForDSL(compiler *dsl.LangSpecCompiler, syntaxFile string) error {
	editorIRConfig := langspeceditor.PushDownAutomatonIRConfigurationCreate[dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole](
		dslTokenScope,
		dsl.LangSpecLexerTokenType.String,
		".lspec",
	)

	editorIRConfig.AddPrototypeTokenRoles(
		dsl.LANG_SPEC_WHITESPACE_ROLE,
		dsl.LANG_SPEC_COMMENT_ROLE,
	)

	editorIRConfig.AddOverride(dsl.TokBlockComment, func(ctx *langspeceditor.TokenOverrideContext) (langspeceditor.StateRule, []langspeceditor.State) {
		openRegex, _ := pattern.LiteralString(runeFactory, "/*").ToRegEx()
		closeRegex, _ := pattern.LiteralString(runeFactory, "*/").ToRegEx()
		return langspeceditor.TokenOverrideDelimitedRegion(ctx,
			openRegex, closeRegex,
			"punctuation.definition.comment.begin",
			ctx.BaseScope,
			"punctuation.definition.comment.end",
		)
	})

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

	editorIRConfig.AddNestOverrideByPredicate(
		func(nest *syntaxa.NestSpec[dsl.LangSpecLexerTokenType]) bool { return nest.OwnerRule == dsl.GrammarIDHeader },
		func(ctx *langspeceditor.NestOverrideContext[dsl.LangSpecLexerTokenType]) (langspeceditor.StateID, []langspeceditor.State) {
			steps := []langspeceditor.NestStep[dsl.LangSpecLexerTokenType]{
				{
					LabelSuffix: "expect_name",
					MetaScope:   "meta.block.header",
					Rules: []langspeceditor.NestStepRule[dsl.LangSpecLexerTokenType]{
						{Token: dsl.TokStringLiteral, Scope: "entity.name.language", Action: langspeceditor.NestRuleActionPushNext},
					},
				},
				{
					LabelSuffix: "expect_version",
					Rules: []langspeceditor.NestStepRule[dsl.LangSpecLexerTokenType]{
						{Token: dsl.TokVersion, Scope: "constant.numeric.version", Action: langspeceditor.NestRuleActionPushNext},
					},
				},
				{
					LabelSuffix: "expect_tail",
					Rules: []langspeceditor.NestStepRule[dsl.LangSpecLexerTokenType]{
						{Token: dsl.TokDashes, Scope: "punctuation.definition.separator", Action: langspeceditor.NestRuleActionPop, PopCount: 3},
						{Token: dsl.TokHeaderSeparator, Scope: "punctuation.section.header", Action: langspeceditor.NestRuleActionMatch},
						{Token: dsl.TokStringLiteral, Scope: "string.quoted.double", Action: langspeceditor.NestRuleActionMatch},
						{Token: dsl.TokKWLSpec, Scope: "keyword.declaration.lspec", Action: langspeceditor.NestRuleActionMatch},
						{Token: dsl.TokVersion, Scope: "constant.numeric.version", Action: langspeceditor.NestRuleActionMatch},
					},
				},
			}
			return langspeceditor.BuildNestStateSequence(ctx, steps)
		},
	)

	lexingRuleSet := dsl.LangSpecCompilerLexingRuleSet(compiler)
	programRule := dsl.LangSpecCompilerProgramRule(compiler)

	editorIR := langspeceditor.PushDownAutomatonIRCreate(
		editorIRConfig,
		lexingRuleSet,
		programRule.GetGrammar().ProducePackage("LangSpec DSL", "0.0.0"),
	)

	return sublime.SublimeTextGenerateSyntaxFile(
		editorIR,
		syntaxFile,
		[]string{".lspec"},
	)
}
