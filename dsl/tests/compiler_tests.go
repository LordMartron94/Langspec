package tests

import (
	"foundation/system"
	"langspec"
	"langspec/bootstrap"
	"langspec/dsl"
	"langspec/dsl/generator"
	dslspec "langspec/dsl/spec"
	langspeceditor "langspec/editor"
	"testing"
)

const testOutput = "libs/langspec/examples/lspec.lspec"
const currentLSpecVersion = "v1.0.0"
const testGenSyntaxOutputFile = "/home/user/.config/sublime-text/Packages/User/LSpec.sublime-syntax"
const testGoBindingsOutputFile = "libs/langspec/examples/generated_go_bindings.go"

// TestMaintainerGenerateLSpecExample regenerates the checked-in example .lspec from the live DSL
// grammar (maintainer workflow; not what end users do).
func TestMaintainerGenerateLSpecExample(t *testing.T) {
	compiler, _, _, teardown := setupTestCompiler()
	defer teardown()

	genCfg := generator.GeneratorConfigCreate[dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecParserNodeKind](
		currentLSpecVersion,
		[]dsl.LangSpecLexerTokenRole{dslspec.LANG_SPEC_COMMENT_ROLE, dslspec.LANG_SPEC_WHITESPACE_ROLE},
	).
		WithTokenFormatter(dsl.LangSpecLexerTokenType.String).
		WithTokenRoleFormatter(dsl.LangSpecLexerTokenRole.String).
		WithNodeKindFormatter(dsl.LangSpecParserNodeKind.String).
		WithEofToken(dslspec.TokEOF).
		WithSublimeConfiguration(
			testGenSyntaxOutputFile,
		).
		WithGoBindingsConfiguration(testGoBindingsOutputFile, "compiler")

	if err := generator.GenerateLSpec(
		dsl.LangSpecCompilerGrammarPackage(compiler),
		convertRulesetToEditor(dsl.LangSpecCompilerLexingRuleSet(compiler)),
		testOutput,
		genCfg,
	); err != nil {
		t.Fatalf("LSpec file generation failed with error: %s", err.Error())
	}
}

// TestParseGeneratedLSpecViaBootstrap regenerates the example .lspec, then builds a LangParser using
// the same bootstrap entry point as clients (CompileParserFromSpec) and parses the file.
func TestParseGeneratedLSpecViaBootstrap(t *testing.T) {
	compiler, scratchAllocFn, sink, teardown := setupTestCompiler()
	defer teardown()

	genCfg := generator.GeneratorConfigCreate[dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecParserNodeKind](
		currentLSpecVersion,
		[]dsl.LangSpecLexerTokenRole{dslspec.LANG_SPEC_COMMENT_ROLE, dslspec.LANG_SPEC_WHITESPACE_ROLE},
	).
		WithTokenFormatter(dsl.LangSpecLexerTokenType.String).
		WithTokenRoleFormatter(dsl.LangSpecLexerTokenRole.String).
		WithNodeKindFormatter(dsl.LangSpecParserNodeKind.String).
		WithEofToken(dslspec.TokEOF).
		WithSublimeConfiguration(
			testGenSyntaxOutputFile,
		).
		WithGoBindingsConfiguration(testGoBindingsOutputFile, "compiler")

	if err := generator.GenerateLSpec(
		dsl.LangSpecCompilerGrammarPackage(compiler),
		convertRulesetToEditor(dsl.LangSpecCompilerLexingRuleSet(compiler)),
		testOutput,
		genCfg,
	); err != nil {
		t.Fatalf("LSpec file generation failed with error: %s", err.Error())
	}

	parser, compiledSym, err := bootstrap.CompileParserFromSpecWithCompiledSymbols(testOutput, scratchAllocFn,
		bootstrap.WithDiagnosticSink(sink),
	)
	if err != nil {
		t.Fatalf("bootstrap.CompileParserFromSpecWithCompiledSymbols failed: %s", err.Error())
	}
	defer langspec.LangParserDestroy(parser)

	session := langspec.LangParserSessionCreate[rune](testOutput, nil)
	contentRune, _ := system.FileReadAllRunes(testOutput)

	trace, rootNode, syntaxErrors, err := langspec.LangParserParseFile(parser, session, nil)

	debugParseResult(false, sink, trace, rootNode, compiledSym)

	if syntaxErrors != nil && syntaxErrors.HasErrors() {
		dsl.RenderSyntaxErrorsWithContext(sink.Writer, contentRune, syntaxErrors, func(r rune, col int) int {
			if r == '\t' {
				return col + 4
			}
			return col + 1
		})
		t.Fatalf("langspec generated-file parse failed with %d syntax errors", len(syntaxErrors.Errors))
	}

	if err != nil {
		t.Fatalf("langspec generated-file parse failed: %s", err.Error())
	}
}

func convertRulesetToEditor(
	ruleset *langspec.LexerRuleset[dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole],
) *langspeceditor.LexingRuleSet[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole] {
	sourceRules := langspec.LexerRulesetGetRules(*ruleset)
	out := make([]langspeceditor.LexingRule[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole], 0, len(sourceRules))
	for _, rule := range sourceRules {
		out = append(out, langspeceditor.LexingRule[rune, dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole]{
			Token:    rule.Token,
			Role:     rule.Role,
			Pattern:  rule.Pattern,
			Priority: rule.Priority,
		})
	}
	return langspeceditor.LexingRuleSetCreate(out...)
}
