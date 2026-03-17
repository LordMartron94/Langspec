package tests

import (
	"foundation/system"
	"langspec"
	"langspec/dsl"
	"langspec/dsl/generator"
	"lexarch"
	"testing"
)

const testOutput = "assets/testing/generated_lspec_spec.lspec"
const currentLSpecVersion = "v0.1.0"
const testGenSyntaxOutputFile = "/home/user/.config/sublime-text/Packages/User/LSpec.sublime-syntax"
const testGoBindingsOutputFile = "assets/testing/generated_go_bindings.go"

func TestDSLCompiler(t *testing.T) {
	compiler, scratchAllocFn, sink, teardown := setupTestCompiler()
	defer teardown()

	genCfg := generator.GeneratorConfigCreate[dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecParserNodeKind](
		currentLSpecVersion,
		[]dsl.LangSpecLexerTokenRole{dsl.LANG_SPEC_COMMENT_ROLE, dsl.LANG_SPEC_WHITESPACE_ROLE},
	).
		WithTokenFormatter(dsl.LangSpecLexerTokenType.String).
		WithTokenRoleFormatter(dsl.LangSpecLexerTokenRole.String).
		WithNodeKindFormatter(dsl.LangSpecParserNodeKind.String).
		WithEofToken(dsl.TokEOF).
		WithSublimeConfiguration(
			testGenSyntaxOutputFile,
		).
		WithGoBindingsConfiguration(testGoBindingsOutputFile, "compiler")

	if err := generator.GenerateLSpec(
		dsl.LangSpecCompilerGrammarPackage(compiler),
		dsl.LangSpecCompilerLexingRuleSet(compiler),
		testOutput,
		genCfg,
	); err != nil {
		t.Fatalf("LSpec file generation failed with error: %s", err.Error())
	}

	result, err := dsl.LangSpecCompilerCompile(compiler, testOutput)
	if err != nil {
		t.Fatalf("Compilation failed with error: %s", err.Error())
	}

	// Toggle to true to see full visual output
	debugCompilation(false, compiler, result, sink)

	result.CompiledLexerSpec.WithCompilationMode(lexarch.Glushkov)
	langParserConfig := langspec.LangParserConfigurationCreate(
		langspec.LangSpecCreate(result.CompiledLexerSpec, result.CompiledParserSpec),
		scratchAllocFn,
	)

	langParser := langspec.LangParserCreate(langParserConfig)
	defer langspec.LangParserDestroy(langParser)

	session := langspec.LangParserSessionCreate[rune](testOutput, nil, false)
	contentRune, _ := system.FileReadAllRunes(testOutput)

	trace, rootNode, syntaxErrors, err := langspec.LangParserParseFile(langParser, session)

	// Toggle to true to see Parse Traces and LST output
	debugParseResult(false, sink, trace, rootNode)

	if syntaxErrors != nil && syntaxErrors.HasErrors() {
		dsl.RenderSyntaxErrorsWithContext(sink.Writer, contentRune, syntaxErrors, lexarch.ColumnAdvanceRune(4))
		t.Fatalf("langspec generated-file parse failed with %d syntax errors", len(syntaxErrors.Errors))
	}

	if err != nil {
		t.Fatalf("langspec generated-file parse failed: %s", err.Error())
	}
}
