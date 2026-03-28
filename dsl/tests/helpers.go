package tests

import (
	"fmt"
	"langspec/dsl"
	"langspec/dsl/semantics"
	"lexarch"
	"memcore"
	"memforge"
	"strconv"
	"syntaxa"
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
	trace *syntaxa.ParseTrace[uint32],
	rootNode *syntaxa.SyntaxaLSTNode[rune, uint32, uint32, uint32],
	sym *semantics.CompiledSymbolTable,
) {
	if !enable {
		return
	}

	dsl.RenderParseTrace(sink.Writer, trace, func(t uint32) string { return strconv.FormatUint(uint64(t), 10) })

	if rootNode != nil {
		lstDump := rootNode.DebugDump(
			syntaxa.LSTDebugFormatter[rune, uint32, uint32, uint32]{
				FormatKind: func(k uint32) string {
					if sym != nil {
						return sym.NodeKindName(k)
					}
					return strconv.FormatUint(uint64(k), 10)
				},
				FormatToken:     func(l lexarch.Lexeme[rune, uint32, uint32]) string { return string(l.Raw) },
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
