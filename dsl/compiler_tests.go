package dsl

import (
	"fmt"
	"foundation/system"
	"langspec"
	"lexarch"
	"memarch"
	"memcore"
	"memforge"
	"syntaxa"
	"testing"
)

const TestFile = "assets/testing/input.lspec"

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

	sink := DefaultLangSpecDiagnosticSink()

	compilerConfig := LangSpecCompilerConfigurationCreate(
		scratchAllocFn,
		nil,
	).WithDiagnosticSink(sink)

	compiler := LangSpecCompilerCreate(compilerConfig)
	defer LangSpecCompilerDestroy(compiler)

	LangSpecCompilerDebugGrammar(compiler)

	result, err := LangSpecCompilerCompile(compiler, TestFile)

	LangSpecCompilerDebugResult(compiler, result, &CompilerDebugConfig{
		DebugParseTrace: false,
		DebugLST:        true,
	})

	if err != nil {
		t.Fatalf("Compilation failed with error: %s", err.Error())
	}

	// testResult(t, result, scratchAllocFn, TestFile, sink)
}

func testResult(
	t *testing.T,
	result *LangSpecCompileResult,
	scratchAllocationFn memarch.AllocationFn,
	sourceFile string,
	sink *LangSpecDiagnosticSink,
) {
	grammarPackage := result.CompiledGrammarPackage
	programRule := grammarPackage.Grammars[grammarPackage.EntryRule]

	grammarDump := programRule.DebugDump(
		syntaxa.GrammarDebugFormatter[string]{
			FormatKind: syntaxa.GrammarKind.String,
			FormatToken: func(s string) string {
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

	grammarPackageDump := grammarPackage.DebugDump(syntaxa.GrammarPackageDebugFormatter[rune, string, string, string, string]{
		FormatToken: func(s string) string {
			return s
		},
	})

	renderGrammarDumps(sink.Writer, grammarDump, grammarPackageDump)

	spec := langspec.LangSpecCreate(
		result.CompiledLexerSpec,
		result.CompiledParserSpec,
	)

	langParserConfiguration := langspec.LangParserConfigurationCreate(spec, scratchAllocationFn)
	langParser := langspec.LangParserCreate(langParserConfiguration)
	defer langspec.LangParserDestroy(langParser)

	session := langspec.LangParserSessionCreate[rune](sourceFile, nil, false)

	_, rootNode, syntaxErrors, err := langspec.LangParserParseFile(
		langParser,
		session,
	)

	// renderParseTrace(sink.Writer, trace, func(t string) string { return t })

	contentRune, _ := system.FileReadAllRunes(sourceFile)

	if syntaxErrors.HasErrors() {
		renderSyntaxErrorsWithContext(sink.Writer, contentRune, syntaxErrors)

		t.Fatalf("file parse failed with %d syntax errors",
			len(syntaxErrors.Errors))
	}

	if err != nil {
		t.Fatalf("testing compiled artifact failed with error: %s", err.Error())
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

	renderLSTDump(sink.Writer, lstDump)
}
