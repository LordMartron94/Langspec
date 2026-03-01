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

func dslNestActionToEditor(a dsl.DSLNestAction) langspeceditor.NestRuleAction {
	switch a {
	case dsl.DSLNestActionMatch:
		return langspeceditor.NestRuleActionMatch
	case dsl.DSLNestActionPushNext:
		return langspeceditor.NestRuleActionPushNext
	case dsl.DSLNestActionPop:
		return langspeceditor.NestRuleActionPop
	default:
		return langspeceditor.NestRuleActionMatch
	}
}

func dslHeaderSpecToNestSteps(spec []dsl.DSLHeaderNestStep, scopeResolver func(dsl.LangSpecLexerTokenType) string) []langspeceditor.NestStep[dsl.LangSpecLexerTokenType] {
	out := make([]langspeceditor.NestStep[dsl.LangSpecLexerTokenType], 0, len(spec))
	for _, step := range spec {
		var rules []langspeceditor.NestStepRule[dsl.LangSpecLexerTokenType]
		for _, e := range step.Expectations {
			for _, tok := range e.Tokens {
				scope := e.ScopeOverride
				if scope == "" {
					scope = scopeResolver(tok)
				}
				rules = append(rules, langspeceditor.NestStepRule[dsl.LangSpecLexerTokenType]{
					Token:    tok,
					Scope:    scope,
					Action:   dslNestActionToEditor(e.NestAction),
					PopCount: e.PopCount,
				})
			}
		}
		out = append(out, langspeceditor.NestStep[dsl.LangSpecLexerTokenType]{
			LabelSuffix: step.LabelSuffix,
			MetaScope:   step.MetaScope,
			Rules:       rules,
		})
	}
	return out
}

/*
BuildSublimeSyntaxForDSL generates a Sublime Text syntax definition file for the
LangSpec DSL from the given compiler. It builds an editor IR with token overrides
(delimited block comments, line comments with capture) and a nest override for the
HEADER production, then writes the result to syntaxFile.

Use this from tools or tests that need .lspec syntax highlighting; the core
langspec/dsl compiler does not depend on editor or sublime.
*/
func BuildSublimeSyntaxForDSL(compiler *dsl.LangSpecCompiler, syntaxFile string) error {
	scopeMap := dsl.LangSpecCompilerScopeMap(compiler)
	scopeResolver := func(t dsl.LangSpecLexerTokenType) string { return scopeMap[t] }
	headerSpec := dsl.LangSpecCompilerHeaderSpec(compiler)

	editorIRConfig := langspeceditor.PushDownAutomatonIRConfigurationCreate[dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole](
		scopeResolver,
		dsl.LangSpecLexerTokenType.String,
		".lspec",
	)

	editorIRConfig.AddPrototypeTokenRoles(
		dsl.LANG_SPEC_WHITESPACE_ROLE,
		dsl.LANG_SPEC_COMMENT_ROLE,
	)

	patternSectionID := dsl.LangSpecGrammarIDFromNode(dsl.NodePatternSection, "")
	editorIRConfig.AddExtraNestIncludes(patternSectionID, dsl.TokPipe, dsl.TokConcat)

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

	editorIRConfig.AddNestOverrideByPredicate(
		func(nest *syntaxa.NestSpec[dsl.LangSpecLexerTokenType]) bool {
			return nest.OwnerRule == dsl.LangSpecGrammarIDFromNode(dsl.NodeHeader, "")
		},
		func(ctx *langspeceditor.NestOverrideContext[dsl.LangSpecLexerTokenType]) (langspeceditor.StateID, []langspeceditor.State) {
			steps := dslHeaderSpecToNestSteps(headerSpec, scopeResolver)
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
