package tests

import (
	"fmt"
	"langspec/dsl"
	"langspec/dsl/semantics"
	"lexarch"
	"memcore"
	"memforge"
	"strconv"
	"strings"
	"syntaxa"
	"syntaxa/lowering"
	"testing"
)

// setupTestCompiler returns the compiler, alloc tools, and a deterministic teardown func.
func setupTestCompiler() (
	compiler *dsl.LangSpecCompiler,
	scratchAllocFn func(sizeBytes, alignment uint64) memcore.MarkRaw,
	diagnosticSink *dsl.LangSpecDiagnosticSink,
	teardownFn func(),
) {
	scratchAllocator := memforge.DynamicLinearAllocatorCreateFunction(uint64(memcore.KiloByte), func(currentCap, neededCap uint64) uint64 {
		newSize := max(currentCap*2, neededCap)
		if newSize > uint64(memcore.GigaByte) {
			panic("too much memory for a test")
		}
		return newSize
	})

	scratchAllocFn = func(sizeBytes, alignment uint64) memcore.MarkRaw {
		return memforge.DynamicLinearAllocatorMallocUnsafe(scratchAllocator, sizeBytes, alignment)
	}

	sink := dsl.DefaultLangSpecDiagnosticSink()

	compilerConfig := dsl.LangSpecCompilerConfigurationCreate(
		scratchAllocFn,
		nil,
	).WithDiagnosticSink(sink)

	compiler = dsl.LangSpecCompilerCreate(compilerConfig)

	teardown := func() {
		dsl.LangSpecCompilerDestroy(compiler)
		memforge.DynamicLinearAllocatorDestroy(scratchAllocator)
	}

	return compiler, scratchAllocFn, sink, teardown
}

// debugParseResult formats parse output from bootstrap (compiled target: uint32 token/role/kind).
// sym should be compileResult.CompiledSymbols from the same compile when non-nil so LST node kinds
// show target-language names, not LangSpecParserNodeKind stringer output.
func debugParseResult(
	enable bool,
	sink *dsl.LangSpecDiagnosticSink,
	trace *syntaxa.ParseTrace,
	rootNode *syntaxa.SyntaxaLSTNode[uint32],
	sym *semantics.CompiledSymbolTable,
) {
	if !enable {
		return
	}

	dsl.RenderParseTrace(sink.Writer, trace, func(t lexarch.TokenKind) string { return strconv.FormatUint(uint64(t), 10) })

	if rootNode != nil {
		lstDump := rootNode.DebugDump(
			syntaxa.LSTDebugFormatter[uint32]{
				FormatKind: func(k uint32) string {
					if sym != nil {
						return sym.NodeKindName(k)
					}
					return strconv.FormatUint(uint64(k), 10)
				},
				FormatToken:     func(l syntaxa.Lexeme) string { return string(l.Raw) },
				FormatAttribute: func(k string, v any) string { return fmt.Sprintf("%s=%v", k, v) },
				ShowTokens:      true,
				ShowAttributes:  true,
				ShowByteSpan:    true,
				ShowLineSpan:    true,
				ShowNodeID:      true,
				ShowRevision:    false,
				SlotPrefix:      "@",
			},
		)
		dsl.RenderLSTDump(sink.Writer, lstDump)
	}
}

// logCompileFailureDiagnostics writes validation findings, then either a lowered grammar-package
// dump (when compileTree ran) or a full LangSpec LST dump (parse succeeded but validation or
// lowering did not complete). Use after bootstrap compile failure.
func logCompileFailureDiagnostics(t *testing.T, result *dsl.LangSpecCompileResult) {
	t.Helper()
	if result == nil {
		t.Logf("compile failure diagnostic: nil LangSpecCompileResult")
		return
	}

	if result.ValidationEntries != nil {
		for _, stage := range result.ValidationEntries.Results {
			for _, e := range stage.Entries {
				t.Logf("validation [%s] %s %s: %s", stage.StageName, e.Severity.String(), e.Code, e.Message)
			}
		}
	}

	pkg := result.CompiledGrammarPackage
	if pkg.Root != nil || len(pkg.SortedGrammarLabels) > 0 {
		sym := result.CompiledSymbols
		formatTok := func(tok lexarch.TokenKind) string {
			if sym != nil {
				return sym.TokenName(uint32(tok))
			}
			return strconv.FormatUint(uint64(tok), 10)
		}
		dump := pkg.DebugDump(
			syntaxa.GrammarPackageDebugFormatter[uint32]{
				FormatToken: formatTok,
			},
			func() *syntaxa.GrammarAnalysis { return lowering.GetAnalysis(&pkg) },
		)
		t.Logf("compile failure diagnostic: lowered grammar package dump:\n%s", dump)
		return
	}

	if result.RootNode != nil {
		sourceText := string(result.SourceRunes)
		lstDump := result.RootNode.DebugDump(
			syntaxa.LSTDebugFormatter[dsl.LangSpecParserNodeKind]{
				FormatKind:       func(k dsl.LangSpecParserNodeKind) string { return k.String() },
				FormatToken:      func(l syntaxa.Lexeme) string { return string(l.Raw) },
				FormatAttribute:  func(k string, v any) string { return fmt.Sprintf("%s=%v", k, v) },
				ShowTokens:       true,
				ShowAttributes:   true,
				ShowByteSpan:     true,
				ShowLineSpan:     true,
				LineSpanSource:   sourceText,
				LineSpanTabWidth: 4,
				ShowNodeID:       true,
				ShowRevision:     false,
				SlotPrefix:       "@",
			},
		)
		var sb strings.Builder
		dsl.RenderLSTDump(&sb, lstDump)
		t.Logf("compile failure diagnostic: no lowered grammar yet (validation failed before lowering); LangSpec LST dump follows (PARSE rules appear under NodeParseSection / NodeParseRule):\n%s", sb.String())
		return
	}

	t.Logf("compile failure diagnostic: no LST root and no grammar package to dump")
}
