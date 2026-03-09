package tests

import (
	"fmt"
	"foundation/system"
	"langspec"
	"langspec/dsl"
	"langspec/dsl/generator"

	"lexarch"
	"syntaxa"
	"testing"

	"memcore"
	"memforge"
)

const TestFile = "assets/testing/input.lspec"
const TestOutput = "assets/testing/generated_lspec_spec.lspec"
const TestOutput2 = "assets/testing/generated_lspec_spec2.lspec"

const currentLSpecVersion = "v0.1.0"

func TestDSLCompiler(t *testing.T) {
	scratchAllocator := memforge.DynamicLinearAllocatorCreateFunction(uint64(memcore.KiloByte), func(currentCap, neededCap uint64) uint64 {
		newSize := max(currentCap*2, neededCap)

		if newSize > uint64(memcore.GigaByte) {
			panic("too much memory for a test")
		}

		return newSize
	})
	defer memforge.DynamicLinearAllocatorDestroy(scratchAllocator)

	scratchAllocFn := func(sizeBytes, alignment uint64) memcore.MarkRaw {
		return memforge.DynamicLinearAllocatorMallocUnsafe(scratchAllocator, sizeBytes, alignment)
	}

	sink := dsl.DefaultLangSpecDiagnosticSink()

	compilerConfig := dsl.LangSpecCompilerConfigurationCreate(
		scratchAllocFn,
		nil,
	).WithDiagnosticSink(sink)

	compiler := dsl.LangSpecCompilerCreate(compilerConfig)
	defer dsl.LangSpecCompilerDestroy(compiler)

	// dsl.LangSpecCompilerDebugGrammar(compiler)

	scopeMap := dsl.LangSpecCompilerScopeMap(compiler)
	genCfg := generator.GeneratorConfigCreate[dsl.LangSpecLexerTokenType, dsl.LangSpecLexerTokenRole, dsl.LangSpecParserNodeKind](
		currentLSpecVersion,
		[]dsl.LangSpecLexerTokenRole{dsl.LANG_SPEC_COMMENT_ROLE, dsl.LANG_SPEC_WHITESPACE_ROLE},
	).
		WithEnableSublime(true).
		WithTokenFormatter(dsl.LangSpecLexerTokenType.String).
		WithTokenRoleFormatter(dsl.LangSpecLexerTokenRole.String).
		WithNodeKindFormatter(dsl.LangSpecParserNodeKind.String).
		WithBaseScopes(scopeMap).
		WithEofToken(dsl.TokEOF)

	if err := generator.GenerateLSpec(
		dsl.LangSpecCompilerGrammarPackage(compiler),
		dsl.LangSpecCompilerLexingRuleSet(compiler),
		TestOutput,
		genCfg,
	); err != nil {
		t.Fatalf("LSpec file generation failed with error: %s", err.Error())
	}

	result, err := dsl.LangSpecCompilerCompile(compiler, TestOutput)

	dsl.LangSpecCompilerDebugResult(compiler, result, &dsl.CompilerDebugConfig{
		DebugParseTrace: false,
		DebugLST:        false,
	})

	if err != nil {
		t.Fatalf("Compilation failed with error: %s", err.Error())
	}

	result.CompiledLexerSpec.WithCompilationMode(lexarch.Glushkov)
	langParserConfig := langspec.LangParserConfigurationCreate(
		langspec.LangSpecCreate(
			result.CompiledLexerSpec,
			result.CompiledParserSpec,
		),
		scratchAllocFn,
	)

	langParser := langspec.LangParserCreate(langParserConfig)
	defer langspec.LangParserDestroy(langParser)

	session := langspec.LangParserSessionCreate[rune](TestOutput, nil, false)
	contentRune, _ := system.FileReadAllRunes(TestOutput)

	grammarDump := result.CompiledGrammarPackage.EntryRuleParserRule.GetGrammar().DebugDump(
		syntaxa.GrammarDebugFormatter[string, string]{
			FormatKind: syntaxa.GrammarKind.String,
			FormatToken: func(s string) string {
				return s
			},
			FormatOutputNodeKind: func(s string) string {
				return s
			},
			FormatRange: func(min int, max *int) string {
				if max == nil {
					return fmt.Sprintf("[%d..∞]", min)
				}
				return fmt.Sprintf("[%d..%d]", min, *max)
			},
			FormatGrammarLabel: func(label syntaxa.GrammarLabel) string {
				return "(" + string(label) + ")"
			},
		},
	)

	grammarPackageDump := result.CompiledGrammarPackage.DebugDump(syntaxa.GrammarPackageDebugFormatter[rune, string, string, string, string]{
		FormatToken: func(s string) string {
			return s
		},
	})

	dsl.RenderGrammarDumps(sink.Writer, grammarDump, grammarPackageDump)

	_, rootNode, syntaxErrors, err := langspec.LangParserParseFile(langParser, session)

	// dsl.RenderParseTrace(sink.Writer, trace, func(t string) string { return t })

	if syntaxErrors != nil && syntaxErrors.HasErrors() {
		dsl.RenderSyntaxErrorsWithContext(sink.Writer, contentRune, syntaxErrors, lexarch.ColumnAdvanceRune(4))
		t.Fatalf(
			"langspec generated-file parse failed with %d syntax errors",
			len(syntaxErrors.Errors),
		)
	}

	if err != nil {
		t.Fatalf(
			"langspec generated-file parse failed: %s",
			err.Error(),
		)
	}

	lstDump := rootNode.DebugDump(
		syntaxa.LSTDebugFormatter[
			rune,
			string,
			string,
			string,
		]{
			FormatKind: func(k string) string {
				return k
			},

			FormatToken: func(l lexarch.Lexeme[
				rune,
				string,
				string,
			]) string {
				return string(l.Raw)
			},

			FormatAttribute: func(k string, v any) string {
				return fmt.Sprintf("%s=%v", k, v)
			},

			/* ───── visual toggles ───── */

			ShowTokens:     true,
			ShowAttributes: true,

			ShowByteSpan: true,
			ShowLineSpan: true,

			ShowNodeID:   true,
			ShowRevision: false,

			SlotPrefix: "@",

			/* colors disabled for now */
			ColorKind:      nil,
			ColorToken:     nil,
			ColorSpan:      nil,
			ColorAttribute: nil,
		},
	)

	dsl.RenderLSTDump(sink.Writer, lstDump)
}
